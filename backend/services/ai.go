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

// extractMarkdownReport extracts the final Markdown report from GLM reasoning output.
// GLM flash models always include chain-of-thought in reasoning_content.
// Strategy: find the last "## Resumo" header and extract everything from there.
// If not found, try to find the largest contiguous block of Portuguese text.
func extractMarkdownReport(text string) string {
	lines := strings.Split(text, "\n")

	// Strategy 1: Find the LAST "## Resumo" line (possibly indented)
	reportStart := -1
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## Resumo") {
			reportStart = i
			break
		}
	}

	if reportStart >= 0 {
		return cleanExtractedLines(lines[reportStart:])
	}

	// Strategy 2: Find last block starting with any "## " header
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") && !strings.Contains(trimmed, "Analyze") && !strings.Contains(trimmed, "Review") {
			return cleanExtractedLines(lines[i:])
		}
	}

	// Strategy 3: Look for "Final" section markers typical of GLM reasoning
	// e.g., "5. **Final" or "**Final Output**" or "**Resultado Final**"
	for i := len(lines) - 1; i >= 0; i-- {
		lower := strings.ToLower(strings.TrimSpace(lines[i]))
		if strings.Contains(lower, "final output") || strings.Contains(lower, "resultado final") ||
			strings.Contains(lower, "relatório final") || strings.Contains(lower, "relatorio final") {
			// The report content starts on the NEXT line (or a few lines after)
			for j := i + 1; j < len(lines); j++ {
				trimmed := strings.TrimSpace(lines[j])
				if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "# ") {
					return cleanExtractedLines(lines[j:])
				}
			}
		}
	}

	return text
}

// cleanExtractedLines removes indentation from extracted report lines
func cleanExtractedLines(lines []string) string {
	var sb strings.Builder
	for _, line := range lines {
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
		sb.WriteString(cleaned)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

// IsAvailable checks if the AI client is configured.
func (c *AIClient) IsAvailable() bool {
	return c != nil && c.apiKey != ""
}
