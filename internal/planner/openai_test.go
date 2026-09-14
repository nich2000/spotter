package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"spotter/internal/model"
)

func TestOpenAIUsesConfiguredBaseURLForResponsesAPI(t *testing.T) {
	var requestPath string
	var requestModel string
	var requestInstructions string
	var requestMaxOutputTokens int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		var payload struct {
			Model           string `json:"model"`
			Instructions    string `json:"instructions"`
			MaxOutputTokens int    `json:"max_output_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requestModel = payload.Model
		requestInstructions = payload.Instructions
		requestMaxOutputTokens = payload.MaxOutputTokens
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output_text":"{\"summary\":\"ok\",\"blocks\":[\"b\"],\"risks\":[\"r\"],\"focus\":[\"f\"]}"}`))
	}))
	defer server.Close()

	planner := OpenAI{
		APIKey:   "ollama",
		BaseURL:  server.URL + "/v1",
		Model:    "gpt-5.2",
		Fallback: RuleBased{},
		Client:   server.Client(),
	}

	plan, err := planner.Generate(context.Background(), model.AppState{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if requestPath != "/v1/responses" {
		t.Fatalf("request path = %q, want %q", requestPath, "/v1/responses")
	}
	if requestModel != "gpt-5.2" {
		t.Fatalf("request model = %q, want %q", requestModel, "gpt-5.2")
	}
	if strings.HasPrefix(requestInstructions, "/no_think\n") {
		t.Fatalf("request instructions = %q, want no qwen no_think prefix", requestInstructions)
	}
	if requestMaxOutputTokens < 1600 {
		t.Fatalf("max_output_tokens = %d, want at least 1600", requestMaxOutputTokens)
	}
	if plan.Summary != "ok" {
		t.Fatalf("plan summary = %q, want %q", plan.Summary, "ok")
	}
}

func TestOpenAIUsesOllamaChatForQwen3(t *testing.T) {
	var requestPath string
	var requestModel string
	var requestStream bool
	var requestThink bool
	var requestRaw []byte
	var requestFormat map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		requestRaw = body
		var payload struct {
			Model  string         `json:"model"`
			Stream bool           `json:"stream"`
			Think  bool           `json:"think"`
			Format map[string]any `json:"format"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requestModel = payload.Model
		requestStream = payload.Stream
		requestThink = payload.Think
		requestFormat = payload.Format
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"{\"summary\":\"ok\",\"blocks\":[\"b\"],\"risks\":[\"r\"],\"focus\":[\"f\"]}"}}`))
	}))
	defer server.Close()

	planner := OpenAI{
		APIKey:   "ollama",
		BaseURL:  server.URL + "/v1",
		Model:    "qwen3:4b",
		Fallback: RuleBased{},
		Client:   server.Client(),
	}

	plan, trace, err := planner.GenerateTrace(context.Background(), model.AppState{})
	if err != nil {
		t.Fatalf("GenerateTrace() error = %v", err)
	}

	if requestPath != "/api/chat" {
		t.Fatalf("request path = %q, want %q", requestPath, "/api/chat")
	}
	if requestModel != "qwen3:4b" {
		t.Fatalf("request model = %q, want %q", requestModel, "qwen3:4b")
	}
	if requestStream {
		t.Fatalf("request stream = true, want false")
	}
	if requestThink {
		t.Fatalf("request think = true, want false")
	}
	if !bytes.Contains(requestRaw, []byte(`"think":false`)) {
		t.Fatalf("request raw = %s, want explicit think:false", string(requestRaw))
	}
	if requestFormat["type"] != "object" {
		t.Fatalf("request format type = %v, want object", requestFormat["type"])
	}
	if plan.Summary != "ok" {
		t.Fatalf("plan summary = %q, want ok", plan.Summary)
	}
	if trace.Model != "qwen3:4b" {
		t.Fatalf("trace model = %q, want qwen3:4b", trace.Model)
	}
	if trace.Prompt == "" {
		t.Fatalf("trace prompt is empty")
	}
	if trace.Response == "" {
		t.Fatalf("trace response is empty")
	}
}

func TestParseDailyPlanTextExtractsFinalJSONAfterThinking(t *testing.T) {
	text := `Проверяю пример {"summary":"example","blocks":[],"risks":[],"focus":[]}.
</think>

{"summary":"ok","blocks":["b"],"risks":["r"],"focus":["f"]}`

	plan, err := parseDailyPlanText(text)
	if err != nil {
		t.Fatalf("parseDailyPlanText() error = %v", err)
	}

	if plan.Summary != "ok" {
		t.Fatalf("plan summary = %q, want ok", plan.Summary)
	}
	if len(plan.Blocks) != 1 || plan.Blocks[0] != "b" {
		t.Fatalf("plan blocks = %+v, want [b]", plan.Blocks)
	}
}

func TestOpenAIUsesOnlyModelPlanWhenModelReturnsEmptyLists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"{\"summary\":\"model only\",\"blocks\":[],\"risks\":[],\"focus\":[]}"}}`))
	}))
	defer server.Close()

	planner := OpenAI{
		APIKey:   "ollama",
		BaseURL:  server.URL + "/v1",
		Model:    "qwen3:4b",
		Fallback: RuleBased{},
		Client:   server.Client(),
	}

	plan, err := planner.Generate(context.Background(), model.AppState{
		Mail: []model.MailMessage{{Subject: "unread", IsUnread: true}},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if plan.Summary != "model only" {
		t.Fatalf("summary = %q, want model only", plan.Summary)
	}
	if len(plan.Blocks) != 0 {
		t.Fatalf("blocks = %+v, want model-provided empty list", plan.Blocks)
	}
	if len(plan.Risks) != 0 {
		t.Fatalf("risks = %+v, want model-provided empty list", plan.Risks)
	}
	if len(plan.Focus) != 0 {
		t.Fatalf("focus = %+v, want model-provided empty list", plan.Focus)
	}
}
