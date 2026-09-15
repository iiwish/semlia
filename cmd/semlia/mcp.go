package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/iiwish/semlia/internal/platform/config"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type credentialTransport struct{ token string }

func (t credentialTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.Header = request.Header.Clone()
	copy.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(copy)
}

func machineClientConfig(lookup config.LookupEnv) (endpoint, token string, err error) {
	base, _ := lookup("SEMLIA_API_URL")
	workspace, _ := lookup("SEMLIA_WORKSPACE_ID")
	token, _ = lookup("SEMLIA_API_TOKEN")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || token == "" {
		return "", "", errors.New("machine client configuration is invalid")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1")) {
		return "", "", errors.New("machine client requires HTTPS")
	}
	if _, err := identity.ParseWorkspaceID(workspace); err != nil {
		return "", "", errors.New("machine workspace is invalid")
	}
	return strings.TrimRight(base, "/") + "/api/v1/workspaces/" + workspace + "/", token, nil
}

func runMCP(ctx context.Context, args []string, lookup config.LookupEnv, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: semlia mcp")
		return 2
	}
	endpoint, token, err := machineClientConfig(lookup)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "semlia-stdio", Version: "0.2.0"}, nil)
	remote, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint + "mcp", HTTPClient: &http.Client{Transport: credentialTransport{token}, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		fmt.Fprintln(stderr, "MCP connection refused")
		return 1
	}
	defer remote.Close()
	listed, err := remote.ListTools(ctx, nil)
	if err != nil {
		fmt.Fprintln(stderr, "MCP tools unavailable")
		return 1
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "semlia", Version: "0.2.0"}, nil)
	for _, tool := range listed.Tools {
		name := tool.Name
		server.AddTool(tool, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result, err := remote.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: request.Params.Arguments})
			if err != nil {
				return nil, errors.New("MCP request refused")
			}
			return result, nil
		})
	}
	resources, err := remote.ListResources(ctx, nil)
	if err != nil {
		fmt.Fprintln(stderr, "MCP resources unavailable")
		return 1
	}
	for _, resource := range resources.Resources {
		uri := resource.URI
		server.AddResource(resource, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			result, err := remote.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
			if err != nil {
				return nil, errors.New("MCP resource refused")
			}
			return result, nil
		})
	}
	if err := server.Run(ctx, &mcp.IOTransport{Reader: io.NopCloser(stdin), Writer: writeCloser{stdout}}); err != nil && ctx.Err() == nil {
		fmt.Fprintln(stderr, "MCP transport stopped")
		return 1
	}
	return 0
}

type writeCloser struct{ io.Writer }

func (writeCloser) Close() error { return nil }
