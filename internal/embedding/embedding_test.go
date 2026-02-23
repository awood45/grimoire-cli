package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaProvider_GenerateEmbedding_success(t *testing.T) {
	expected := []float32{0.1, 0.2, 0.3, 0.4}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/embeddings", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var req ollamaRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "test-model", req.Model)
		assert.Equal(t, "hello world", req.Prompt)

		w.Header().Set("Content-Type", "application/json")
		resp := ollamaResponse{Embedding: expected}
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "test-model")
	result, err := provider.GenerateEmbedding(context.Background(), "hello world")
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestOllamaProvider_GenerateEmbedding_serverError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "test-model")
	_, err := provider.GenerateEmbedding(context.Background(), "hello")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeEmbeddingError))
}

func TestOllamaProvider_GenerateEmbedding_serverDown(t *testing.T) {
	provider := NewOllamaProvider("http://localhost:1", "test-model")
	_, err := provider.GenerateEmbedding(context.Background(), "hello")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeEmbeddingError))
}

func TestOllamaProvider_ModelID(t *testing.T) {
	provider := NewOllamaProvider("http://localhost:11434", "nomic-embed-text")
	assert.Equal(t, "nomic-embed-text", provider.ModelID())
}

func TestNoopProvider_GenerateEmbedding_returnsNil(t *testing.T) {
	provider := &NoopProvider{}
	result, err := provider.GenerateEmbedding(context.Background(), "hello")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestNoopProvider_ModelID(t *testing.T) {
	provider := &NoopProvider{}
	assert.Equal(t, "none", provider.ModelID())
}

func TestOllamaProvider_GenerateEmbedding_invalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "test-model")
	_, err := provider.GenerateEmbedding(context.Background(), "hello")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeEmbeddingError))
}

func TestOllamaProvider_GenerateEmbedding_cancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	provider := NewOllamaProvider(server.URL, "test-model")
	_, err := provider.GenerateEmbedding(ctx, "hello")
	require.Error(t, err)
}

func TestOllamaProvider_GenerateEmbedding_invalidURL(t *testing.T) {
	// A URL with no scheme causes http.NewRequestWithContext to fail.
	provider := NewOllamaProvider("://invalid", "test-model")
	_, err := provider.GenerateEmbedding(context.Background(), "hello")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeEmbeddingError))
}

func TestOllamaProvider_GenerateEmbedding_serverErrorUnreadableBody(t *testing.T) {
	// Server returns non-200 and immediately closes the connection
	// so the body read may yield empty/error content.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "test-model")
	_, err := provider.GenerateEmbedding(context.Background(), "hello")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeEmbeddingError))
}

// errorReadCloser is a ReadCloser whose Read always returns an error.
type errorReadCloser struct{}

func (e *errorReadCloser) Read(_ []byte) (int, error) {
	return 0, errors.New("simulated read error")
}

func (e *errorReadCloser) Close() error { return nil }

// Ensure errorReadCloser implements io.ReadCloser.
var _ io.ReadCloser = (*errorReadCloser)(nil)

// errorBodyTransport returns an HTTP response with an unreadable body.
type errorBodyTransport struct {
	statusCode int
}

func (t *errorBodyTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: t.statusCode,
		Body:       &errorReadCloser{},
	}, nil
}

func TestOllamaProvider_GenerateEmbedding_bodyReadError(t *testing.T) {
	provider := NewOllamaProvider("http://localhost", "test-model")
	// Inject a custom transport that returns a non-200 response with an unreadable body.
	provider.client = &http.Client{Transport: &errorBodyTransport{statusCode: http.StatusInternalServerError}}

	_, err := provider.GenerateEmbedding(context.Background(), "hello")
	require.Error(t, err)
	assert.True(t, sberrors.HasCode(err, sberrors.ErrCodeEmbeddingError))
	assert.Contains(t, err.Error(), "body unreadable")
}
