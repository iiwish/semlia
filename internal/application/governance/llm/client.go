// Package llm holds the provider-client abstraction behind live governed
// generation. One ProviderClient interface covers every protocol; the
// OpenAI-compatible wire format is the v1 implementation (it serves both the
// openai and openai_compatible protocols), while anthropic and gemini are
// registered stubs that fail with the stable provider_unsupported error until
// their adapters land.
//
// Secrets never appear in this package's errors, logs or request metadata:
// the credential is resolved from the persisted environment-variable NAME at
// call time and used exactly once as the bearer token (SSOT §12 S-002).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/domain/governance"
)

// ErrProviderUnsupported marks a provider whose protocol has no live adapter.
var ErrProviderUnsupported = errors.New("provider protocol is not supported")

// ErrProviderUnavailable marks a provider that could not be reached, timed
// out or refused the request with a transport-level failure.
var ErrProviderUnavailable = errors.New("provider is unavailable")

// ProviderUnsupportedError is returned for persisted protocols whose live
// adapters have not landed. The HTTP boundary answers 422 PROVIDER_UNSUPPORTED.
type ProviderUnsupportedError struct {
	Protocol governance.ModelProviderProtocol
}

func (err *ProviderUnsupportedError) Error() string {
	return ErrProviderUnsupported.Error() + ": " + string(err.Protocol)
}

// ProviderUnavailableError is returned when a configured provider cannot
// serve the request. The HTTP boundary answers 503 PROVIDER_UNAVAILABLE.
// Detail carries bounded, credential-free context.
type ProviderUnavailableError struct {
	Detail string
	Err    error
}

func (err *ProviderUnavailableError) Error() string {
	return ErrProviderUnavailable.Error() + ": " + err.Detail
}

func (err *ProviderUnavailableError) Unwrap() error { return err.Err }

// Message is one chat message of the completion request.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompleteRequest is the protocol-neutral completion input. Tools are out of
// scope for v1 governed generation.
type CompleteRequest struct {
	Model     string
	Messages  []Message
	MaxTokens int
}

// Usage carries the provider-reported token counts recorded on the agent run
// as cost (SSOT §8.6).
type Usage struct {
	PromptTokens     int64
	CompletionTokens int64
}

// CompleteResponse is the protocol-neutral completion result.
type CompleteResponse struct {
	Content      string
	Usage        Usage
	FinishReason string
}

// ProviderClient is the single interface every protocol adapter implements.
// Additional protocols are additive adapters behind it.
type ProviderClient interface {
	Protocol() governance.ModelProviderProtocol
	Complete(ctx context.Context, request CompleteRequest) (CompleteResponse, error)
}

// DefaultTimeout bounds every provider call; the request context is honored
// on top of it.
const DefaultTimeout = 60 * time.Second

// CredentialResolver resolves the persisted environment-variable NAME into
// the secret material at call time.
type CredentialResolver func(ctx context.Context, credentialEnv string) (string, error)

// EnvironmentCredentialResolver reads the secret from the process
// environment. A missing variable degrades to provider_unavailable instead of
// escalating privileges or falling back to another credential source.
func EnvironmentCredentialResolver(_ context.Context, credentialEnv string) (string, error) {
	value, ok := os.LookupEnv(credentialEnv)
	if !ok || strings.TrimSpace(value) == "" {
		return "", &ProviderUnavailableError{
			Detail: "credential environment variable is not set",
		}
	}
	return value, nil
}

// NewProviderClient is the additive adapter factory: one case per protocol.
func NewProviderClient(
	provider governance.ModelProvider,
	resolver CredentialResolver,
	httpClient *http.Client,
) (ProviderClient, error) {
	if !provider.Protocol.Valid() {
		return nil, governance.ErrInvalidArgument
	}
	if resolver == nil {
		resolver = EnvironmentCredentialResolver
	}
	switch provider.Protocol {
	case governance.ProtocolOpenAI, governance.ProtocolOpenAICompatible:
		return newOpenAICompatibleClient(provider, resolver, httpClient)
	case governance.ProtocolAnthropic, governance.ProtocolGemini:
		return unsupportedClient{protocol: provider.Protocol}, nil
	default:
		return nil, &ProviderUnsupportedError{Protocol: provider.Protocol}
	}
}

// unsupportedClient is the registered stub for protocols whose adapters land
// in a later packet. Configurations persist; live calls fail explicitly.
type unsupportedClient struct {
	protocol governance.ModelProviderProtocol
}

func (client unsupportedClient) Protocol() governance.ModelProviderProtocol { return client.protocol }

func (client unsupportedClient) Complete(context.Context, CompleteRequest) (CompleteResponse, error) {
	return CompleteResponse{}, &ProviderUnsupportedError{Protocol: client.protocol}
}

// ---------- OpenAI-compatible implementation ----------

const openAIDefaultBaseURL = "https://api.openai.com/v1"

type openAICompatibleClient struct {
	protocol      governance.ModelProviderProtocol
	baseURL       string
	credentialEnv string
	resolver      CredentialResolver
	httpClient    *http.Client
}

func newOpenAICompatibleClient(
	provider governance.ModelProvider,
	resolver CredentialResolver,
	httpClient *http.Client,
) (ProviderClient, error) {
	baseURL := openAIDefaultBaseURL
	if provider.BaseURL != nil && strings.TrimSpace(*provider.BaseURL) != "" {
		baseURL = strings.TrimRight(strings.TrimSpace(*provider.BaseURL), "/")
	} else if provider.Protocol == governance.ProtocolOpenAICompatible {
		return nil, fmt.Errorf("%w: openai_compatible provider requires a base URL", governance.ErrInvalidArgument)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}
	return openAICompatibleClient{
		protocol: provider.Protocol, baseURL: baseURL, credentialEnv: provider.CredentialEnv,
		resolver: resolver, httpClient: httpClient,
	}, nil
}

func (client openAICompatibleClient) Protocol() governance.ModelProviderProtocol {
	return client.protocol
}

type openAIChatRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
}

// Complete posts one chat/completions call with the resolved bearer
// credential and propagates the request context (timeouts included). Any
// transport, credential-resolution or non-2xx failure degrades to the stable
// provider_unavailable error; response bodies are never echoed so provider
// error payloads cannot smuggle content into Semlia responses or logs.
func (client openAICompatibleClient) Complete(ctx context.Context, request CompleteRequest) (CompleteResponse, error) {
	if request.Model == "" || len(request.Messages) == 0 {
		return CompleteResponse{}, governance.ErrInvalidArgument
	}
	credential, err := client.resolver(ctx, client.credentialEnv)
	if err != nil {
		return CompleteResponse{}, err
	}
	body, err := json.Marshal(openAIChatRequest{
		Model: request.Model, Messages: request.Messages, MaxTokens: request.MaxTokens,
	})
	if err != nil {
		return CompleteResponse{}, fmt.Errorf("encode provider request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return CompleteResponse{}, &ProviderUnavailableError{Detail: "provider request could not be built", Err: err}
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+credential)
	httpResponse, err := client.httpClient.Do(httpRequest)
	if err != nil {
		return CompleteResponse{}, &ProviderUnavailableError{Detail: "provider call failed", Err: err}
	}
	defer func() { _ = httpResponse.Body.Close() }()
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(httpResponse.Body, 1<<20))
		return CompleteResponse{}, &ProviderUnavailableError{
			Detail: fmt.Sprintf("provider answered HTTP %d", httpResponse.StatusCode),
		}
	}
	payload, err := io.ReadAll(io.LimitReader(httpResponse.Body, 1<<20))
	if err != nil {
		return CompleteResponse{}, &ProviderUnavailableError{Detail: "provider response could not be read", Err: err}
	}
	var decoded openAIChatResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return CompleteResponse{}, &ProviderUnavailableError{Detail: "provider response is not valid JSON", Err: err}
	}
	if len(decoded.Choices) == 0 {
		return CompleteResponse{}, &ProviderUnavailableError{Detail: "provider response carries no choices"}
	}
	return CompleteResponse{
		Content:      decoded.Choices[0].Message.Content,
		Usage:        Usage{PromptTokens: decoded.Usage.PromptTokens, CompletionTokens: decoded.Usage.CompletionTokens},
		FinishReason: decoded.Choices[0].FinishReason,
	}, nil
}
