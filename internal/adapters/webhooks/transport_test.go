package webhooks

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
)

func TestDestinationPolicyRejectsSSRF(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "::1", "fe80::1", "fc00::1", "::ffff:127.0.0.1", "2001:db8::1", "64:ff9b::7f00:1"} {
		t.Run(address, func(t *testing.T) {
			if publicDestination(netip.MustParseAddr(address)) {
				t.Fatal("SSRF destination allowed")
			}
		})
	}
	client := NewSafeClient(nil)
	for _, endpoint := range []string{"http://example.com/", "https://user:password@example.com/", "https://example.com:8443/", "https://127.0.0.1/", "https://[::1]/"} {
		if err := client.Validate(context.Background(), endpoint); err == nil {
			t.Errorf("unsafe endpoint allowed: %s", endpoint)
		}
	}
}

type changingResolver struct {
	answers [][]netip.Addr
	calls   int
}

func (r *changingResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	index := min(r.calls, len(r.answers)-1)
	r.calls++
	return r.answers[index], nil
}
func TestDNSRebindingIsRevalidatedBeforeSend(t *testing.T) {
	resolver := &changingResolver{answers: [][]netip.Addr{{netip.MustParseAddr("8.8.8.8")}, {netip.MustParseAddr("127.0.0.1")}}}
	client := NewSafeClient(nil)
	client.Resolver = resolver
	if err := client.Validate(context.Background(), "https://receiver.example/"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Send(context.Background(), "https://receiver.example/", []byte("private-body"), nil); !errors.Is(err, ErrDestination) {
		t.Fatal("rebound endpoint was not rejected", err)
	}
	if resolver.calls != 2 {
		t.Fatal("send did not resolve independently")
	}
}
func TestPinnedDialAndRedirectRefusal(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls++ }))
	defer target.Close()
	calls := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Redirect(w, r, target.URL, 307) }))
	defer source.Close()
	parsed, _ := url.Parse(source.URL)
	parsed.Host = "receiver.example:" + parsed.Port()
	resolver := &changingResolver{answers: [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}, {netip.MustParseAddr("10.0.0.1")}}}
	client := NewSafeClient([]netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")})
	client.Resolver = resolver
	status, err := client.Send(context.Background(), parsed.String(), []byte("body"), nil)
	if err != nil || status != 307 || calls != 1 || targetCalls != 0 || resolver.calls != 1 {
		t.Fatalf("redirect/pinning status=%d calls=%d target=%d DNS=%d err=%v", status, calls, targetCalls, resolver.calls, err)
	}
}
func TestMixedPublicAndPrivateDNSAnswersFailClosed(t *testing.T) {
	client := NewSafeClient(nil)
	client.Resolver = &changingResolver{answers: [][]netip.Addr{{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("169.254.169.254")}}}
	if client.Validate(context.Background(), "https://receiver.example/") == nil {
		t.Fatal("private fallback address allowed")
	}
}
