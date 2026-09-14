package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	governance "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/embedding"
)

type Client struct {
	client *http.Client
	lookup func(string) string
}

func NewClient(client *http.Client, lookup func(string) string) *Client {
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	clone := *client
	if clone.Timeout <= 0 {
		clone.Timeout = 45 * time.Second
	}
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if lookup == nil {
		lookup = os.Getenv
	}
	return &Client{client: &clone, lookup: lookup}
}

func (client *Client) Configured(config domain.Config) bool {
	_, valid := client.credential(config)
	return valid
}

func (client *Client) credential(config domain.Config) (string, bool) {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Scheme != "https" && endpoint.Scheme != "http") {
		return "", false
	}
	credential := strings.TrimSpace(client.lookup(config.CredentialEnv))
	return credential, credential != "" && governance.CredentialRevisionDigest(credential) == config.CredentialRevision && config.Dimension > 0 && config.Dimension <= 4096 && config.Model != ""
}

func (client *Client) EmbedBatch(ctx context.Context, config domain.Config, texts []string) ([][]float32, error) {
	credential, configured := client.credential(config)
	if !configured {
		return nil, domain.ErrNotConfigured
	}
	if len(texts) < 1 || len(texts) > domain.BatchSize {
		return nil, domain.ErrInvalid
	}
	for _, text := range texts {
		if len(text) == 0 || len(text) > 8192 {
			return nil, domain.ErrInvalid
		}
	}
	body, _ := json.Marshal(struct {
		Model      string   `json:"model"`
		Input      []string `json:"input"`
		Dimensions int      `json:"dimensions"`
		Encoding   string   `json:"encoding_format"`
	}{config.Model, texts, config.Dimension, "float"})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(config.Endpoint, "/")+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, domain.ErrProvider
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+credential)
	response, err := client.client.Do(request)
	if err != nil {
		return nil, domain.ErrProvider
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, domain.ErrProvider
	}
	var result struct {
		Model string `json:"model"`
		Data  []struct {
			Index     *int              `json:"index"`
			Embedding []json.RawMessage `json:"embedding"`
		} `json:"data"`
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024+1))
	if err != nil || len(data) > 4*1024*1024 || json.Unmarshal(data, &result) != nil || len(result.Data) != len(texts) {
		return nil, domain.ErrProvider
	}
	if result.Model != "" && result.Model != config.Model {
		return nil, domain.ErrProvider
	}
	vectors := make([][]float32, len(texts))
	for _, item := range result.Data {
		if item.Index == nil || *item.Index < 0 || *item.Index >= len(texts) || vectors[*item.Index] != nil {
			return nil, domain.ErrProvider
		}
		vector := make([]float32, len(item.Embedding))
		for i, raw := range item.Embedding {
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &vector[i]) != nil {
				return nil, domain.ErrProvider
			}
		}
		vectors[*item.Index] = vector
	}
	if domain.ValidateVectors(vectors, len(texts), config.Dimension) != nil {
		return nil, domain.ErrProvider
	}
	return vectors, nil
}
