// Package mcp exposes only the canonical released-semantic application surface.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	app "github.com/iiwish/semlia/internal/application/distribution"
	execapp "github.com/iiwish/semlia/internal/application/execution"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/distribution"
	execdomain "github.com/iiwish/semlia/internal/domain/execution"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
)

type resolveArgs struct {
	Query          map[string]any `json:"query"`
	IdempotencyKey string         `json:"idempotencyKey"`
}
type searchArgs struct {
	Text string `json:"text"`
}
type inspectArgs struct {
	ID string `json:"id"`
}

func NewHandler(service *app.Service, executors ...*execapp.Service) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server {
		limit, ok := authapp.CredentialFromContext(request.Context())
		if !ok {
			return nil
		}
		server := mcp.NewServer(&mcp.Implementation{Name: "semlia", Version: "0.2.0"}, nil)
		// A stateless session is built from each already-authenticated HTTP request.
		// No session can retain a credential or grant after next-request revocation.
		prepare := func(ctx context.Context) context.Context { return authapp.WithCredentialLimit(ctx, limit) }
		trace := request.Header.Get("X-Trace-ID")
		if len(executors) > 0 && executors[0] != nil {
			type executeArgs struct {
				PlanID         string `json:"planId"`
				PlanDigest     string `json:"planDigest"`
				IdempotencyKey string `json:"idempotencyKey"`
			}
			mcp.AddTool(server, &mcp.Tool{Name: "semantic_execute", Description: "Execute a persisted plan using bounded read-only PostgreSQL. Result rows are ephemeral; replays return metadata only."}, func(ctx context.Context, _ *mcp.CallToolRequest, args executeArgs) (*mcp.CallToolResult, execdomain.Result, error) {
				plan, err := identity.ParseResolvedSemanticPlanID(args.PlanID)
				if err != nil {
					return toolFailure(domain.ErrInvalidArgument), execdomain.Result{}, nil
				}
				result, err := executors[0].Execute(prepare(ctx), execapp.Request{WorkspaceID: limit.WorkspaceID, PlanID: plan, PlanDigest: args.PlanDigest, IdempotencyKey: args.IdempotencyKey, Channel: "mcp", PrincipalRef: limit.PrincipalID.String(), TraceID: trace})
				if err != nil {
					return toolFailure(err), execdomain.Result{}, nil
				}
				return nil, result, nil
			})
			mcp.AddTool(server, &mcp.Tool{Name: "semantic_execution", Description: "Inspect execution metadata after current authorization checks. Stored rows are never available."}, func(ctx context.Context, _ *mcp.CallToolRequest, args inspectArgs) (*mcp.CallToolResult, execdomain.Result, error) {
				result, err := executors[0].Get(prepare(ctx), limit.WorkspaceID, args.ID, limit.PrincipalID.String(), trace)
				if err != nil {
					return toolFailure(err), execdomain.Result{}, nil
				}
				return nil, result, nil
			})
			mcp.AddTool(server, &mcp.Tool{Name: "semantic_cancel", Description: "Request cancellation of an authorized running execution. Inspect metadata to observe its final outcome."}, func(ctx context.Context, _ *mcp.CallToolRequest, args inspectArgs) (*mcp.CallToolResult, execdomain.Result, error) {
				result, err := executors[0].Cancel(prepare(ctx), limit.WorkspaceID, args.ID, limit.PrincipalID.String(), trace)
				if err != nil {
					return toolFailure(err), execdomain.Result{}, nil
				}
				return nil, result, nil
			})
		}
		for _, name := range []string{"semantic_resolve", "semantic_describe"} {
			mcp.AddTool(server, &mcp.Tool{Name: name, Description: "Resolve immutable released semantics; returns a plan or canonical refusal."}, func(ctx context.Context, call *mcp.CallToolRequest, args resolveArgs) (*mcp.CallToolResult, map[string]any, error) {
				var raw struct {
					Query json.RawMessage `json:"query"`
				}
				if json.Unmarshal(call.Params.Arguments, &raw) != nil {
					return toolFailure(domain.ErrInvalidArgument), nil, nil
				}
				payload := raw.Query
				var query domain.SemanticQueryInput
				decoder := json.NewDecoder(bytes.NewReader(payload))
				decoder.DisallowUnknownFields()
				if decoder.Decode(&query) != nil {
					return toolFailure(domain.ErrInvalidArgument), nil, nil
				}
				if name == "semantic_describe" {
					query.Intent = domain.IntentDescribe
				}
				result, err := service.Resolve(prepare(ctx), app.ResolveRequest{WorkspaceID: limit.WorkspaceID, PrincipalRef: limit.PrincipalID.String(), TraceID: trace, Input: query, Channel: "mcp", IdempotencyKey: args.IdempotencyKey})
				if err != nil {
					return toolFailure(err), nil, nil
				}
				return nil, app.Response(result), nil
			})
		}
		mcp.AddTool(server, &mcp.Tool{Name: "semantic_search", Description: "Search authorized assets in the bound immutable release."}, func(ctx context.Context, _ *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, map[string]any, error) {
			items, err := service.Search(prepare(ctx), app.SearchRequest{WorkspaceID: limit.WorkspaceID, PrincipalRef: limit.PrincipalID.String(), TraceID: trace, Text: args.Text})
			if err != nil {
				return toolFailure(err), nil, nil
			}
			return nil, map[string]any{"items": items}, nil
		})
		mcp.AddTool(server, &mcp.Tool{Name: "semantic_plan", Description: "Inspect a plan with current authorization checks."}, func(ctx context.Context, _ *mcp.CallToolRequest, args inspectArgs) (*mcp.CallToolResult, map[string]any, error) {
			id, err := identity.ParseResolvedSemanticPlanID(args.ID)
			if err != nil {
				return toolFailure(domain.ErrInvalidArgument), nil, nil
			}
			plan, err := service.GetPlan(prepare(ctx), limit.WorkspaceID, id, limit.PrincipalID.String(), trace)
			if err != nil {
				return toolFailure(err), nil, nil
			}
			payload, _ := json.Marshal(app.PublicPlan(plan))
			var value map[string]any
			decoder := json.NewDecoder(bytes.NewReader(payload))
			decoder.UseNumber()
			_ = decoder.Decode(&value)
			return nil, value, nil
		})
		mcp.AddTool(server, &mcp.Tool{Name: "semantic_query", Description: "Inspect a saved resolution or refusal with current authorization checks."}, func(ctx context.Context, _ *mcp.CallToolRequest, args inspectArgs) (*mcp.CallToolResult, map[string]any, error) {
			id, err := identity.ParseSemanticQueryID(args.ID)
			if err != nil {
				return toolFailure(domain.ErrInvalidArgument), nil, nil
			}
			result, err := service.GetQuery(prepare(ctx), limit.WorkspaceID, id, limit.PrincipalID.String(), trace)
			if err != nil {
				return toolFailure(err), nil, nil
			}
			return nil, app.Response(result), nil
		})
		server.AddResource(&mcp.Resource{URI: "semlia://contract", Name: "Semantic contract", MIMEType: "application/json"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			state := "not_configured"
			if len(executors) > 0 && executors[0] != nil {
				state = "requires_execution_validation"
			}
			payload, _ := json.Marshal(map[string]any{"schemaVersion": "1.0.0", "releaseOnly": true, "execution": state})
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "semlia://contract", MIMEType: "application/json", Text: string(payload)}}}, nil
		})
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20, PropagateRequestCancellation: true})
}

func toolFailure(err error) *mcp.CallToolResult {
	code := "INTERNAL_ERROR"
	var denial *auth.DenialError
	var validation *domain.ValidationError
	switch {
	case errors.Is(err, execdomain.ErrInvalidPlan):
		code = "EXECUTION_PLAN_UNSUPPORTED"
	case errors.Is(err, execdomain.ErrConflict), errors.Is(err, execdomain.ErrBusy):
		code = "EXECUTION_CONFLICT"
	case errors.Is(err, execdomain.ErrNotFound):
		code = "NOT_FOUND"
	case errors.As(err, &denial):
		code = string(denial.Decision.ReasonCode)
	case errors.As(err, &validation):
		code = string(validation.Code)
	case errors.Is(err, domain.ErrInvalidArgument):
		code = "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrNotFound):
		code = "NOT_FOUND"
	case errors.Is(err, domain.ErrConflict):
		code = "CONFLICT"
	}
	payload, _ := json.Marshal(map[string]string{"code": code})
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}}, StructuredContent: map[string]string{"code": code}}
}
