package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/iiwish/semlia/internal/platform/config"
	"github.com/iiwish/semlia/pkg/identity"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func runSemantic(ctx context.Context, args []string, lookup config.LookupEnv, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "usage: semlia semantic <describe|search|resolve|plan|query|execute|execution> <query-json|search-text|id>")
		return 2
	}
	base, _ := lookup("SEMLIA_API_URL")
	workspace, _ := lookup("SEMLIA_WORKSPACE_ID")
	token, _ := lookup("SEMLIA_API_TOKEN")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || token == "" {
		fmt.Fprintln(stderr, "semantic client configuration is invalid")
		return 2
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1")) {
		fmt.Fprintln(stderr, "semantic client requires HTTPS")
		return 2
	}
	if _, err := identity.ParseWorkspaceID(workspace); err != nil {
		fmt.Fprintln(stderr, "semantic workspace is invalid")
		return 2
	}
	path := strings.TrimRight(base, "/") + "/api/v1/workspaces/" + workspace + "/"
	method := http.MethodGet
	var body []byte
	switch args[0] {
	case "execute":
		var input struct {
			PlanID         identity.ResolvedSemanticPlanID `json:"planId"`
			PlanDigest     string                          `json:"planDigest"`
			IdempotencyKey string                          `json:"idempotencyKey"`
		}
		decoder := json.NewDecoder(strings.NewReader(args[1]))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || input.PlanID.IsZero() || len(input.PlanDigest) != 71 {
			return 2
		}
		if input.IdempotencyKey == "" {
			input.IdempotencyKey, _ = lookup("SEMLIA_IDEMPOTENCY_KEY")
		}
		if input.IdempotencyKey == "" {
			fmt.Fprintln(stderr, "execution requires an explicit idempotency key")
			return 2
		}
		body, err = json.Marshal(map[string]any{"planId": input.PlanID, "planDigest": input.PlanDigest, "idempotencyKey": input.IdempotencyKey, "channel": "cli"})
		if err != nil {
			return 2
		}
		method = http.MethodPost
		path += "resolved-semantic-plans/" + input.PlanID.String() + ":execute"
	case "cancel":
		if _, err := identity.ParseRunID(args[1]); err != nil {
			return 2
		}
		method = http.MethodPost
		path += "query-executions/" + args[1] + ":cancel"
	case "execution":
		if _, err := identity.ParseRunID(args[1]); err != nil {
			return 2
		}
		path += "query-executions/" + args[1]
	case "search":
		path += "semantic-search?q=" + url.QueryEscape(args[1])
	case "plan":
		if _, err := identity.ParseResolvedSemanticPlanID(args[1]); err != nil {
			return 2
		}
		path += "resolved-semantic-plans/" + args[1]
	case "query":
		if _, err := identity.ParseSemanticQueryID(args[1]); err != nil {
			return 2
		}
		path += "semantic-queries/" + args[1]
	case "resolve", "describe":
		var query map[string]any
		decoder := json.NewDecoder(strings.NewReader(args[1]))
		decoder.UseNumber()
		if decoder.Decode(&query) != nil {
			fmt.Fprintln(stderr, "query must be a JSON object")
			return 2
		}
		key, _ := lookup("SEMLIA_IDEMPOTENCY_KEY")
		if key == "" {
			id, err := identity.NewEventID()
			if err != nil {
				return 1
			}
			key = "cli-" + id.String()
		}
		body, err = json.Marshal(map[string]any{"query": query, "channel": "cli", "idempotencyKey": key})
		if err != nil {
			return 2
		}
		method = http.MethodPost
		if args[0] == "describe" {
			path += "semantic-describe"
		} else {
			path += "semantic-queries:resolve"
		}
	default:
		fmt.Fprintln(stderr, "unsupported semantic command")
		return 2
	}
	request, err := http.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	if err != nil {
		return 2
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		fmt.Fprintln(stderr, "semantic request failed")
		return 1
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil || !json.Valid(payload) {
		fmt.Fprintln(stderr, "semantic response invalid")
		return 1
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		fmt.Fprintf(stderr, "semantic request refused (HTTP %d)\n", response.StatusCode)
		_, _ = stdout.Write(payload)
		fmt.Fprintln(stdout)
		return 1
	}
	_, _ = stdout.Write(payload)
	fmt.Fprintln(stdout)
	if args[0] == "execute" || args[0] == "execution" {
		var result struct {
			Run struct {
				State string `json:"state"`
			} `json:"run"`
		}
		if json.Unmarshal(payload, &result) != nil {
			return 1
		}
		if result.Run.State == "failed" || result.Run.State == "cancelled" || result.Run.State == "unknown" {
			return 1
		}
	}
	return 0
}
