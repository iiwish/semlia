package webhooks

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var ErrDestination = errors.New("webhook destination is not allowed")

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}
type SafeClient struct {
	Resolver     Resolver
	testNetworks []netip.Prefix
}

// Explicit networks are a local transport-test policy, never deployment input.
func NewSafeClient(testNetworks []netip.Prefix) *SafeClient {
	return &SafeClient{Resolver: net.DefaultResolver, testNetworks: append([]netip.Prefix(nil), testNetworks...)}
}
func publicDestination(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20"} {
		if netip.MustParsePrefix(block).Contains(ip) {
			return false
		}
	}
	return ip.Is4() || netip.MustParsePrefix("2000::/3").Contains(ip)
}
func (c *SafeClient) resolve(ctx context.Context, endpoint string) (*url.URL, []netip.Addr, error) {
	u, err := url.Parse(endpoint)
	if err != nil || len(endpoint) > 2048 || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || strings.Contains(u.Hostname(), "%") {
		return nil, nil, ErrDestination
	}
	if len(c.testNetworks) == 0 && (u.Scheme != "https" || u.Port() != "" && u.Port() != "443") {
		return nil, nil, ErrDestination
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, nil, ErrDestination
	}
	addresses, err := c.Resolver.LookupNetIP(ctx, "ip", u.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, nil, ErrDestination
	}
	for _, address := range addresses {
		allowed := publicDestination(address)
		for _, network := range c.testNetworks {
			if network.Contains(address.Unmap()) {
				allowed = true
			}
		}
		if !allowed {
			return nil, nil, ErrDestination
		}
	}
	return u, addresses, nil
}
func (c *SafeClient) Validate(ctx context.Context, endpoint string) error {
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, _, err := c.resolve(bounded, endpoint)
	return err
}
func (c *SafeClient) Send(ctx context.Context, endpoint string, body []byte, headers map[string]string) (int, error) {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	u, ips, err := c.resolve(bounded, endpoint)
	if err != nil {
		return 0, err
	}
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	// DNS is validated once and the dialer receives only that pinned IP. Neither
	// environment proxies nor redirect following can re-resolve a destination.
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}, DisableKeepAlives: true, ResponseHeaderTimeout: 5 * time.Second,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(bounded, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, ErrDestination
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, errors.New("webhook transport failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode, nil
}
