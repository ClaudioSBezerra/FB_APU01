package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Claude API Models
const (
	ModelHaiku  = "claude-haiku-4-5-20251001"
	ModelSonnet = "claude-sonnet-4-5-20250929"
)

// AIClient wraps communication with the Anthropic Claude API.
type AIClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// Claude API request/response types
type claudeRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	Messages  []claudeMessage  `json:"messages"`
	System    string           `json:"system,omitempty"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// AIResponse is the result returned to handlers.
type AIResponse struct {
	Text         string `json:"text"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Model        string `json:"model"`
}

// NewAIClient creates an AIClient from environment config.
func NewAIClient() *AIClient {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil
	}
	return &AIClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.anthropic.com/v1/messages",
	}
}

// Generate sends a prompt to Claude and returns the narrative text.
// Uses retry with exponential backoff (max 3 attempts).
func (c *AIClient) Generate(system, userPrompt, model string, maxTokens int) (*AIResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("AI client not configured (ANTHROPIC_API_KEY not set)")
	}
	if model == "" {
		model = ModelHaiku
	}
	if maxTokens == 0 {
		maxTokens = 2048
	}

	reqBody := claudeRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    system,
		Messages: []claudeMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := c.doRequest(reqBody)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt < 3 {
			backoff := time.Duration(attempt*attempt) * time.Second
			time.Sleep(backoff)
		}
	}
	return nil, fmt.Errorf("AI API failed after 3 attempts: %w", lastErr)
}

func (c *AIClient) doRequest(reqBody claudeRequest) (*AIResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if claudeResp.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", claudeResp.Error.Type, claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	return &AIResponse{
		Text:         claudeResp.Content[0].Text,
		InputTokens:  claudeResp.Usage.InputTokens,
		OutputTokens: claudeResp.Usage.OutputTokens,
		Model:        reqBody.Model,
	}, nil
}

// IsAvailable checks if the AI client is configured.
func (c *AIClient) IsAvailable() bool {
	return c != nil && c.apiKey != ""
}
