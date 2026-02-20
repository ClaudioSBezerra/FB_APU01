package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Z.AI GLM Models
const (
	ModelFlash         = "glm-4.7-flash" // Free tier - primary
	ModelFlashFallback = "glm-4.5-flash" // Free tier - fallback for rate limits
)

// AIClient wraps communication with the Z.AI GLM API (OpenAI-compatible).
type AIClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// OpenAI-compatible request/response types (used by Z.AI)
type chatRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Messages  []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Code    string `json:"code"`
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
	apiKey := os.Getenv("ZAI_API_KEY")
	if apiKey == "" {
		return nil
	}
	return &AIClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		baseURL: "https://api.z.ai/api/paas/v4/chat/completions",
	}
}

// GenerateFastRaw is like GenerateFast but returns the raw AI text without
// running extractMarkdownReport. Use this for SQL generation and other tasks
// where the output is not a Markdown report (e.g. code blocks).
func (c *AIClient) GenerateFastRaw(system, userPrompt, model string, maxTokens int) (*AIResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("AI client not configured")
	}
	if model == "" {
		model = ModelFlash
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}
	fastClient := &http.Client{Timeout: 25 * time.Second}
	origClient := c.httpClient
	c.httpClient = fastClient
	defer func() { c.httpClient = origClient }()

	messages := []chatMessage{{Role: "user", Content: userPrompt}}
	if system != "" {
		messages = append([]chatMessage{{Role: "system", Content: system}}, messages...)
	}
	reqBody := chatRequest{Model: model, MaxTokens: maxTokens, Messages: messages}

	resp, err := c.doRequestRaw(reqBody)
	if err != nil {
		if strings.Contains(err.Error(), "429") && reqBody.Model == ModelFlash {
			fmt.Printf("[AI Raw] Rate limited on %s, single attempt with %s\n", ModelFlash, ModelFlashFallback)
			reqBody.Model = ModelFlashFallback
			resp, err = c.doRequestRaw(reqBody)
		}
	}
	return resp, err
}

// GenerateFast is like Generate but with a single attempt and shorter timeout.
// Use this in HTTP handlers where the user is waiting — the background worker
// handles retries. Returns an error immediately on rate-limit or timeout.
func (c *AIClient) GenerateFast(system, userPrompt, model string, maxTokens int) (*AIResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("AI client not configured")
	}
	if model == "" {
		model = ModelFlash
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}
	// Shorter HTTP timeout for the synchronous request path
	fastClient := &http.Client{Timeout: 25 * time.Second}
	origClient := c.httpClient
	c.httpClient = fastClient
	defer func() { c.httpClient = origClient }()

	messages := []chatMessage{{Role: "user", Content: userPrompt}}
	if system != "" {
		messages = append([]chatMessage{{Role: "system", Content: system}}, messages...)
	}
	reqBody := chatRequest{Model: model, MaxTokens: maxTokens, Messages: messages}

	// Single attempt — no retries
	resp, err := c.doRequest(reqBody)
	if err != nil {
		// On rate-limit try fallback model once, but don't loop
		if strings.Contains(err.Error(), "429") && reqBody.Model == ModelFlash {
			fmt.Printf("[AI Fast] Rate limited on %s, single attempt with %s\n", ModelFlash, ModelFlashFallback)
			reqBody.Model = ModelFlashFallback
			resp, err = c.doRequest(reqBody)
		}
	}
	return resp, err
}

// Generate sends a prompt to Z.AI GLM and returns the narrative text.
// Uses retry with exponential backoff (max 3 attempts).
func (c *AIClient) Generate(system, userPrompt, model string, maxTokens int) (*AIResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("AI client not configured (ZAI_API_KEY not set)")
	}
	if model == "" {
		model = ModelFlash
	}
	if maxTokens == 0 {
		maxTokens = 4096
	}

	// OpenAI-compatible format: system prompt goes as a message with role "system"
	messages := []chatMessage{
		{Role: "user", Content: userPrompt},
	}
	if system != "" {
		messages = append([]chatMessage{{Role: "system", Content: system}}, messages...)
	}

	reqBody := chatRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := c.doRequest(reqBody)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		// On rate limit (429), switch to fallback model immediately
		if strings.Contains(err.Error(), "429") && reqBody.Model == ModelFlash {
			fmt.Printf("[AI] Rate limited on %s, switching to fallback %s\n", ModelFlash, ModelFlashFallback)
			reqBody.Model = ModelFlashFallback
			time.Sleep(2 * time.Second)
			continue
		}

		if attempt < 3 {
			backoff := time.Duration(attempt*2) * time.Second
			time.Sleep(backoff)
		}
	}
	return nil, fmt.Errorf("AI API failed after 3 attempts: %w", lastErr)
}

func (c *AIClient) doRequest(reqBody chatRequest) (*AIResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

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

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", chatResp.Error.Code, chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	// GLM models may return text in reasoning_content instead of content
	text := chatResp.Choices[0].Message.Content
	if text == "" {
		text = chatResp.Choices[0].Message.ReasoningContent
	}
	if text == "" {
		return nil, fmt.Errorf("empty response from API")
	}

	// GLM flash models include chain-of-thought in reasoning_content.
	// Extract only the final Markdown report (starts with "## ").
	text = extractMarkdownReport(text)

	return &AIResponse{
		Text:         text,
		InputTokens:  chatResp.Usage.PromptTokens,
		OutputTokens: chatResp.Usage.CompletionTokens,
		Model:        reqBody.Model,
	}, nil
}

// doRequestRaw is identical to doRequest but skips extractMarkdownReport.
// Returns content + reasoning_content concatenated so callers can search for
// code blocks (e.g. ```sql) anywhere in the full response.
func (c *AIClient) doRequestRaw(reqBody chatRequest) (*AIResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

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

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", chatResp.Error.Code, chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	reasoning := strings.TrimSpace(chatResp.Choices[0].Message.ReasoningContent)

	// Strategy for SQL generation:
	// 1. Prefer content (final answer) — it has the actual SQL code block.
	// 2. If content is empty, fall back to reasoning_content.
	// 3. Never mix them: reasoning contains prose "with `x`" that breaks SQL extraction.
	var text string
	switch {
	case content != "" && strings.Contains(content, "```"):
		// content has a code block — use it directly
		text = content
	case content != "":
		// content has text but no code block — still prefer it over reasoning
		text = content
	case reasoning != "":
		// content empty — fall back to reasoning_content
		text = reasoning
	default:
		return nil, fmt.Errorf("empty response from API")
	}

	return &AIResponse{
		Text:         text,
		InputTokens:  chatResp.Usage.PromptTokens,
		OutputTokens: chatResp.Usage.CompletionTokens,
		Model:        reqBody.Model,
	}, nil
}

// extractMarkdownReport extracts the final Markdown report from GLM reasoning output.
// GLM flash models return chain-of-thought in reasoning_content with the actual
// report embedded (often indented). This function finds the best candidate block
// starting with "## Resumo" that has enough content to be a real report.
func extractMarkdownReport(text string) string {
	lines := strings.Split(text, "\n")

	// Find ALL occurrences of "## Resumo" and pick the best one
	// (the one followed by the most content lines)
	type candidate struct {
		startIdx int
		content  string
	}
	var candidates []candidate

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Resumo") {
			extracted := cleanExtractedLines(lines[i:])
			candidates = append(candidates, candidate{startIdx: i, content: extracted})
		}
	}

	// Pick the candidate with the longest content (most likely the full report)
	if len(candidates) > 0 {
		best := candidates[0]
		for _, c := range candidates[1:] {
			if len(c.content) > len(best.content) {
				best = c
			}
		}
		// Only use if it has meaningful content (> 200 chars = likely a real report)
		if len(best.content) > 200 {
			return best.content
		}
	}

	// Fallback: look for the longest block starting with any "## " header
	// that contains Portuguese fiscal keywords
	ptKeywords := []string{"ICMS", "faturamento", "recolher", "tributari", "imposto", "IBS", "CBS"}
	bestBlock := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			block := cleanExtractedLines(lines[i:])
			// Check if it contains Portuguese fiscal content
			keywordCount := 0
			blockLower := strings.ToLower(block)
			for _, kw := range ptKeywords {
				if strings.Contains(blockLower, strings.ToLower(kw)) {
					keywordCount++
				}
			}
			if keywordCount >= 3 && len(block) > len(bestBlock) {
				bestBlock = block
			}
		}
	}
	if bestBlock != "" {
		return bestBlock
	}

	return text
}

// cleanExtractedLines removes indentation and trailing reasoning from extracted lines.
// Stops extraction when it encounters a new numbered reasoning step (e.g., "5. **Review")
// which indicates we've gone past the report back into chain-of-thought.
func cleanExtractedLines(lines []string) string {
	var sb strings.Builder
	foundContent := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Stop if we hit a new numbered reasoning step AFTER finding report content
		// Pattern: line starts with a digit followed by ". " (e.g., "5. **Review")
		if foundContent && len(trimmed) > 3 {
			if trimmed[0] >= '1' && trimmed[0] <= '9' && (strings.HasPrefix(trimmed[1:], ". **") || strings.HasPrefix(trimmed[1:], ".  **")) {
				break
			}
		}

		// Remove leading indentation (up to 8 spaces from chain-of-thought nesting)
		cleaned := line
		spaces := 0
		for _, c := range cleaned {
			if c == ' ' {
				spaces++
			} else {
				break
			}
		}
		if spaces > 0 && spaces <= 8 {
			cleaned = cleaned[spaces:]
		}

		if strings.TrimSpace(cleaned) != "" {
			foundContent = true
		}

		sb.WriteString(cleaned)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

// IsAvailable checks if the AI client is configured.
func (c *AIClient) IsAvailable() bool {
	return c != nil && c.apiKey != ""
}
