package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/awood45/grimoire-cli/internal/sberrors"
)

// OllamaProvider generates embeddings via the Ollama HTTP API.
type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

// Compile-time interface check.
var _ Provider = (*OllamaProvider)(nil)

// NewOllamaProvider creates a provider that calls the Ollama embeddings API.
func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	return &OllamaProvider{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{},
	}
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaResponse struct {
	Embedding []float32 `json:"embedding"`
}

// GenerateEmbedding calls the Ollama API to generate a vector embedding.
func (p *OllamaProvider) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(ollamaRequest{Model: p.model, Prompt: text})
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeEmbeddingError, "failed to marshal request")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeEmbeddingError, "failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeEmbeddingError, "failed to call Ollama API")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, sberrors.Newf(sberrors.ErrCodeEmbeddingError, "Ollama API returned status %d (body unreadable)", resp.StatusCode)
		}
		return nil, sberrors.Newf(sberrors.ErrCodeEmbeddingError, "Ollama API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeEmbeddingError, "failed to decode response")
	}

	return result.Embedding, nil
}

// ModelID returns the configured model name.
func (p *OllamaProvider) ModelID() string {
	return p.model
}
