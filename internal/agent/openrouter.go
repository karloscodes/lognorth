package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"
)

// openRouterAPIBase is a var so tests can point it at httptest.
var openRouterAPIBase = "https://openrouter.ai/api/v1"

// DefaultOpenRouterModel is used when no model is chosen.
const DefaultOpenRouterModel = "anthropic/claude-sonnet-4.6"

// OpenRouterModels is the curated set offered in Settings. Coding-capable
// models, Anthropic first. Slugs match OpenRouter's catalog. Edit to add more.
var OpenRouterModels = []string{
	"anthropic/claude-sonnet-4.6",
	"anthropic/claude-opus-4.8",
	"anthropic/claude-haiku-4.5",
	"openai/gpt-5.5",
	"openai/gpt-5.3-codex",
	"qwen/qwen3.7-max",
	"qwen/qwen3.7-plus",
	"moonshotai/kimi-k2.7-code",
	"moonshotai/kimi-k2.6",
	"deepseek/deepseek-v4-pro",
	"deepseek/deepseek-v4-flash",
	"google/gemini-3.1-pro-preview",
	"google/gemini-3.5-flash",
}

// IsKnownModel reports whether slug is one we offer.
func IsKnownModel(slug string) bool {
	return slices.Contains(OpenRouterModels, slug)
}

// ValidateOpenRouterKey confirms the key works and returns its label, so we
// never store a key OpenRouter would reject at fix time.
func ValidateOpenRouterKey(ctx context.Context, key string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterAPIBase+"/key", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter /key: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var out struct {
		Data struct {
			Label string `json:"label"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	return out.Data.Label, nil
}
