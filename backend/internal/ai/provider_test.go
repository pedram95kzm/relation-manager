package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"relations/internal/config"
	"relations/internal/database"
	"relations/internal/platform"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCredentialEncryption(t *testing.T) {
	master := []byte(strings.Repeat("k", 32))
	secret := "sensitive-provider-key-123"
	first, err := encryptKey(master, secret)
	if err != nil {
		t.Fatal(err)
	}
	second, err := encryptKey(master, secret)
	if err != nil || string(first) == string(second) || strings.Contains(string(first), secret) {
		t.Fatal("nonce reuse or plaintext storage")
	}
	if plain, err := decryptKey(master, first); err != nil || plain != secret {
		t.Fatal("round trip", err)
	}
	first[len(first)-1] ^= 1
	if _, err := decryptKey(master, first); err == nil {
		t.Fatal("accepted modified ciphertext")
	}
	if _, err := decryptKey([]byte(strings.Repeat("x", 32)), second); err == nil {
		t.Fatal("accepted wrong master key")
	}
	if _, err := decryptKey(master, []byte{1}); err == nil {
		t.Fatal("accepted truncated ciphertext")
	}
	if _, err := encryptKey([]byte(strings.Repeat("k", 16)), secret); err == nil {
		t.Fatal("accepted short encryption key")
	}
}

func TestProviderProtocolsAndUsage(t *testing.T) {
	for _, provider := range []string{"openai", "deepseek", "anthropic", "gemini"} {
		t.Run(provider, func(t *testing.T) {
			service := New(config.Config{}, nil)
			service.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.String(), "secret") || req.URL.Scheme != "https" {
					t.Fatal("key in URL or insecure destination")
				}
				data, _ := io.ReadAll(req.Body)
				var payload map[string]any
				if json.Unmarshal(data, &payload) != nil {
					t.Fatal("bad request JSON")
				}
				response := ""
				switch provider {
				case "openai", "deepseek":
					if req.Header.Get("Authorization") != "Bearer secret-key-123" || len(payload["messages"].([]any)) != 2 {
						t.Fatal("chat auth/instructions")
					}
					format := payload["response_format"].(map[string]any)
					if provider == "openai" && (format["type"] != "json_schema" || payload["max_completion_tokens"] != float64(512) || payload["store"] != false) {
						t.Fatal("OpenAI schema or bounds")
					}
					if provider == "deepseek" && (format["type"] != "json_object" || payload["max_tokens"] != float64(512)) {
						t.Fatal("DeepSeek JSON or bounds")
					}
					response = `{"choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":23,"completion_tokens":11}}`
				case "anthropic":
					if req.Header.Get("x-api-key") != "secret-key-123" || req.Header.Get("anthropic-version") != "2023-06-01" || payload["system"] != "trusted instructions" || payload["output_config"] == nil {
						t.Fatal("Anthropic auth/schema/instructions")
					}
					response = `{"stop_reason":"end_turn","content":[{"type":"text","text":"{\"ok\":true}"}],"usage":{"input_tokens":23,"output_tokens":11}}`
				case "gemini":
					if req.Header.Get("x-goog-api-key") != "secret-key-123" || !strings.HasSuffix(req.URL.Path, "sample-model:generateContent") || payload["systemInstruction"] == nil {
						t.Fatal("Gemini auth/instructions")
					}
					format := payload["generationConfig"].(map[string]any)["responseFormat"].(map[string]any)["text"].(map[string]any)
					if format["mimeType"] != "application/json" || format["schema"] == nil {
						t.Fatal("Gemini schema")
					}
					response = `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"hidden reasoning","thought":true},{"text":"{\"ok\":true}"}]}}],"usageMetadata":{"promptTokenCount":23,"candidatesTokenCount":11}}`
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response)), Header: http.Header{}}, nil
			})
			result, err := service.requestWithRetry(context.Background(), database.ProviderConfig{Provider: provider, Model: "sample-model", MaxTokens: 512, TimeoutSeconds: 1}, "secret-key-123", "trusted instructions", "untrusted data", object(map[string]any{"ok": map[string]any{"type": "boolean"}}))
			if err != nil || string(result.Content) != `{"ok":true}` || result.InputTokens != 23 || result.OutputTokens != 11 {
				t.Fatal(result, err)
			}
		})
	}
}

func TestProviderFailuresAreBoundedAndSanitized(t *testing.T) {
	service := New(config.Config{}, nil)
	count := 0
	service.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		count++
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("secret-key-123 private-personal-context")), Header: http.Header{}}, nil
	})
	_, err := service.requestWithRetry(context.Background(), database.ProviderConfig{Provider: "openai", Model: "sample-model", MaxTokens: 512, TimeoutSeconds: 1, Retries: 3}, "secret-key-123", "trusted", "private-personal-context", schema())
	var appErr *platform.Error
	if count != 1 || !errors.As(err, &appErr) || appErr.Code != "ai_provider_error" || strings.Contains(appErr.Message, "secret") || strings.Contains(appErr.Message, "private") {
		t.Fatal("unsafe error or retried authentication failure", err, count)
	}
	service.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		count++
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	started := time.Now()
	_, err = service.requestWithRetry(context.Background(), database.ProviderConfig{Provider: "openai", Model: "sample-model", MaxTokens: 512, TimeoutSeconds: 1, Retries: 3}, "key", "trusted", "data", schema())
	if !errors.As(err, &appErr) || appErr.Code != "ai_timeout" || time.Since(started) > 2*time.Second || count != 2 {
		t.Fatal("timeout not total budget", err, count)
	}
}

func TestProviderDoesNotAcceptTruncationOrRefusal(t *testing.T) {
	for _, response := range []string{
		`{"choices":[{"finish_reason":"length","message":{"content":"{\"ok\":true}"}}]}`,
		`{"choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}","refusal":"declined"}}]}`,
		`{"choices":[{"finish_reason":"stop","message":{"content":""}}]}`,
	} {
		service := New(config.Config{}, nil)
		service.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response)), Header: http.Header{}}, nil
		})
		if _, err := service.requestWithRetry(context.Background(), database.ProviderConfig{Provider: "openai", Model: "sample-model", MaxTokens: 512, TimeoutSeconds: 1}, "key", "trusted", "data", schema()); err == nil {
			t.Fatal("accepted incomplete/refused response")
		}
	}
}

func TestProviderRetriesTransientErrorsAndBlocksRedirects(t *testing.T) {
	service := New(config.Config{}, nil)
	attempts := 0
	service.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		status := 503
		body := `{"private":"do not expose"}`
		if attempts == 2 {
			status = 200
			body = `{"choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}]}`
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	if _, err := service.requestWithRetry(context.Background(), database.ProviderConfig{Provider: "openai", Model: "sample-model", MaxTokens: 512, TimeoutSeconds: 2, Retries: 1}, "key", "trusted", "data", schema()); err != nil || attempts != 2 {
		t.Fatal("transient retry", attempts, err)
	}
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true; w.WriteHeader(200) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer origin.Close()
	service = New(config.Config{}, nil)
	service.endpoints["openai"] = origin.URL
	if _, err := service.requestWithRetry(context.Background(), database.ProviderConfig{Provider: "openai", Model: "sample-model", MaxTokens: 512, TimeoutSeconds: 2, Retries: 0}, "secret-key", "trusted", "personal-data", schema()); err == nil || redirected {
		t.Fatal("followed redirect with sensitive data")
	}
}
