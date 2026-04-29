package semantic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// OpenAIEmbedder calls any OpenAI-compatible /v1/embeddings endpoint.
// Works with OpenAI, Voyage AI, Mistral, Ollama, LiteLLM, vLLM, etc.
type OpenAIEmbedder struct {
	baseURL string
	apiKey  string
	model   string
	dim     int
	client  *http.Client
}

// OpenAIConfig holds configuration for the OpenAI embedder.
type OpenAIConfig struct {
	BaseURL string // default: https://api.openai.com
	APIKey  string // from SBDB_EMBED_API_KEY env
	Model   string // default: text-embedding-3-small
	Dim     int    // default: 1536 (model-dependent)
}

// NewOpenAIEmbedder creates an embedder that calls an OpenAI-compatible API.
func NewOpenAIEmbedder(cfg OpenAIConfig) (*OpenAIEmbedder, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = os.Getenv("SBDB_EMBED_BASE_URL")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}

	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("SBDB_EMBED_API_KEY")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("embedding API key not set: use SBDB_EMBED_API_KEY env or config")
	}

	if cfg.Model == "" {
		cfg.Model = os.Getenv("SBDB_EMBED_MODEL")
	}
	if cfg.Model == "" {
		cfg.Model = "text-embedding-3-small"
	}

	if cfg.Dim == 0 {
		cfg.Dim = 1536
	}

	return &OpenAIEmbedder{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		dim:     cfg.Dim,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (e *OpenAIEmbedder) ModelID() string { return e.model }
func (e *OpenAIEmbedder) Dim() int        { return e.dim }

// Embed sends texts to the /v1/embeddings endpoint and returns vectors.
func (e *OpenAIEmbedder) Embed(texts []string) ([][]float32, error) {
	reqBody := openAIEmbedRequest{
		Model: e.model,
		Input: texts,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling embed request: %w", err)
	}

	url := e.baseURL + "/v1/embeddings"
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed API call failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed API returned %d: %s", resp.StatusCode, string(body))
	}

	var result openAIEmbedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing embed response: %w", err)
	}

	// Update dim from actual response
	if len(result.Data) > 0 && len(result.Data[0].Embedding) > 0 {
		e.dim = len(result.Data[0].Embedding)
	}

	vectors := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vectors[i] = d.Embedding
	}

	return vectors, nil
}

type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []openAIEmbedData `json:"data"`
}

type openAIEmbedData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}
