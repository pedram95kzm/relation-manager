package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"relations/internal/database"
	"relations/internal/platform"
)

type providerResult struct {
	Content                   []byte
	InputTokens, OutputTokens int64
}

type providerFailure struct {
	code  string
	retry bool
}

func (e *providerFailure) Error() string { return e.code }

// Production URLs are fixed. Administrators cannot direct credentials or sensitive data to arbitrary hosts.
var endpoints = map[string]string{
	"openai":    "https://api.openai.com/v1/chat/completions",
	"deepseek":  "https://api.deepseek.com/chat/completions",
	"anthropic": "https://api.anthropic.com/v1/messages",
	"gemini":    "https://generativelanguage.googleapis.com/v1beta/models/",
}

func providerError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return platform.Err(504, "ai_timeout", "The provider request timed out. Your deterministic scores remain available.")
	}
	var failure *providerFailure
	if errors.As(err, &failure) && failure.code == "ai_rate_limited" {
		return platform.Err(503, "ai_rate_limited", "The provider is temporarily busy. Your deterministic scores remain available.")
	}
	return platform.Err(502, "ai_provider_error", "The provider could not generate a report. Check the provider settings or try again later.")
}

func (s *Service) request(ctx context.Context, cfg database.ProviderConfig, key, system, user string, outputSchema map[string]any) (providerResult, error) {
	endpoint := s.endpoints[cfg.Provider]
	body := map[string]any{}
	headers := map[string]string{}
	messages := []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}}
	switch cfg.Provider {
	case "openai":
		body = map[string]any{"model": cfg.Model, "messages": messages, "max_completion_tokens": cfg.MaxTokens, "store": false, "response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "relations_report", "strict": true, "schema": outputSchema}}}
		headers["Authorization"] = "Bearer " + key
	case "deepseek":
		body = map[string]any{"model": cfg.Model, "messages": messages, "max_tokens": cfg.MaxTokens, "response_format": map[string]any{"type": "json_object"}}
		headers["Authorization"] = "Bearer " + key
	case "anthropic":
		body = map[string]any{"model": cfg.Model, "system": system, "messages": []map[string]string{{"role": "user", "content": user}}, "max_tokens": cfg.MaxTokens, "output_config": map[string]any{"format": map[string]any{"type": "json_schema", "schema": outputSchema}}}
		headers["x-api-key"] = key
		headers["anthropic-version"] = "2023-06-01"
	case "gemini":
		endpoint += url.PathEscape(cfg.Model) + ":generateContent"
		body = map[string]any{"systemInstruction": map[string]any{"parts": []map[string]string{{"text": system}}}, "contents": []map[string]any{{"role": "user", "parts": []map[string]string{{"text": user}}}}, "generationConfig": map[string]any{"maxOutputTokens": cfg.MaxTokens, "responseFormat": map[string]any{"text": map[string]any{"mimeType": "application/json", "schema": outputSchema}}}}
		headers["x-goog-api-key"] = key
	default:
		return providerResult{}, &providerFailure{"ai_provider_error", false}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return providerResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return providerResult{}, &providerFailure{"ai_provider_error", false}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return providerResult{}, ctx.Err()
		}
		return providerResult{}, &providerFailure{"ai_provider_error", true}
	}
	defer resp.Body.Close()
	// Never inspect, return or log provider error text: it can contain secrets or supplied context.
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == 429 {
			return providerResult{}, &providerFailure{"ai_rate_limited", true}
		}
		return providerResult{}, &providerFailure{"ai_provider_error", resp.StatusCode >= 500}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return providerResult{}, &providerFailure{"ai_provider_error", false}
	}
	switch cfg.Provider {
	case "openai", "deepseek":
		var response struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Content string  `json:"content"`
					Refusal *string `json:"refusal"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				Prompt     int64 `json:"prompt_tokens"`
				Completion int64 `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(data, &response) != nil || len(response.Choices) != 1 || response.Choices[0].FinishReason != "stop" || response.Choices[0].Message.Refusal != nil {
			return providerResult{}, &providerFailure{"ai_provider_error", false}
		}
		return checkedResult([]byte(response.Choices[0].Message.Content), response.Usage.Prompt, response.Usage.Completion)
	case "anthropic":
		var response struct {
			StopReason string                        `json:"stop_reason"`
			Content    []struct{ Type, Text string } `json:"content"`
			Usage      struct {
				Input  int64 `json:"input_tokens"`
				Output int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(data, &response) != nil || response.StopReason != "end_turn" {
			return providerResult{}, &providerFailure{"ai_provider_error", false}
		}
		var content strings.Builder
		for _, block := range response.Content {
			if block.Type == "text" {
				content.WriteString(block.Text)
			}
		}
		return checkedResult([]byte(content.String()), response.Usage.Input, response.Usage.Output)
	case "gemini":
		var response struct {
			Candidates []struct {
				FinishReason string `json:"finishReason"`
				Content      struct {
					Parts []struct {
						Text    string `json:"text"`
						Thought bool   `json:"thought"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
			Usage struct {
				Input  int64 `json:"promptTokenCount"`
				Output int64 `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		}
		if json.Unmarshal(data, &response) != nil || len(response.Candidates) != 1 || response.Candidates[0].FinishReason != "STOP" {
			return providerResult{}, &providerFailure{"ai_provider_error", false}
		}
		var content strings.Builder
		for _, part := range response.Candidates[0].Content.Parts {
			if !part.Thought {
				content.WriteString(part.Text)
			}
		}
		return checkedResult([]byte(content.String()), response.Usage.Input, response.Usage.Output)
	}
	return providerResult{}, &providerFailure{"ai_provider_error", false}
}

func checkedResult(content []byte, input, output int64) (providerResult, error) {
	if len(bytes.TrimSpace(content)) == 0 || len(content) > 65536 || input < 0 || output < 0 {
		return providerResult{}, &providerFailure{"ai_provider_error", false}
	}
	return providerResult{content, input, output}, nil
}

func (s *Service) requestWithRetry(ctx context.Context, cfg database.ProviderConfig, key, system, user string, outputSchema map[string]any) (providerResult, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	for attempt := 0; ; attempt++ {
		result, err := s.request(ctx, cfg, key, system, user, outputSchema)
		if err == nil {
			return result, nil
		}
		var failure *providerFailure
		if attempt >= cfg.Retries || !errors.As(err, &failure) || !failure.retry {
			return result, providerError(err)
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return providerResult{}, providerError(ctx.Err())
		case <-timer.C:
		}
	}
}
