package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"spotter/internal/model"
)

type diagnosticTransport func(*http.Request) (*http.Response, error)

func (f diagnosticTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestModelDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, body, stage string
		status            int
		failed            bool
	}{
		{"success", `{"done":true,"done_reason":"stop","eval_count":12,"prompt_eval_count":34,"load_duration":1000000,"message":{"content":"{\"summary\":\"ok\",\"blocks\":[],\"risks\":[],\"focus\":[]}"}}`, "complete", 200, false},
		{"truncated", `{"done":true,"done_reason":"length","eval_count":2000,"message":{"content":"{\"blocks\":[\"no\""}}`, "generation", 200, true},
		{"invalid_json", `{"message":{"content":"broken"}}`, "validate_plan", 200, true},
		{"invalid_shape", `{"message":{"content":"{\"blocks\":[\"no\",\"no\"]}"}}`, "validate_plan", 200, true},
		{"http_error", `model not found`, "http_status", 404, true},
		{"bad_envelope", `not json`, "decode_response", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			p := OpenAI{APIKey: "secret-key", BaseURL: "http://localhost:11434/v1", Model: "qwen3:4b", Logger: slog.New(slog.NewJSONHandler(&logs, nil)), Client: &http.Client{Transport: diagnosticTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}}
			ctx, id := WithOperation(context.Background())
			plan, trace, err := p.GenerateTrace(ctx, model.AppState{Notes: []model.Note{{Title: "private-note"}}})
			if err != nil {
				t.Fatal(err)
			}
			if trace.Stage != tc.stage || trace.HTTPStatus != tc.status || trace.OperationID != id || (trace.Error != "") != tc.failed {
				t.Fatalf("unexpected trace: %+v", trace)
			}
			if trace.ResponseBody != tc.body || !json.Valid(trace.Request) {
				t.Fatal("missing request/response evidence")
			}
			if tc.failed && plan.Summary != "Spotter рекомендации недоступны." {
				t.Fatalf("failure not visible: %+v", plan)
			}
			if !tc.failed && (trace.OutputTokens != 12 || trace.PromptTokens != 34 || trace.LoadDurationNS != 1000000) {
				t.Fatalf("metrics lost: %+v", trace)
			}
			if strings.Contains(logs.String(), "secret-key") || strings.Contains(logs.String(), "private-note") || !strings.Contains(logs.String(), id) {
				t.Fatalf("unsafe or uncorrelated log: %s", logs.String())
			}
		})
	}
}

func TestModelTimeoutDiagnostics(t *testing.T) {
	p := OpenAI{APIKey: "ollama", Model: "qwen3:4b", Timeout: time.Millisecond, Client: &http.Client{Transport: diagnosticTransport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}}
	_, trace, err := p.GenerateTrace(context.Background(), model.AppState{})
	if err != nil || trace.Status != "failed" || trace.Stage != "request" || !strings.Contains(trace.Error, "context deadline exceeded") {
		t.Fatalf("timeout trace: %+v, %v", trace, err)
	}
}

func TestRejectInvalidPlans(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `{"summary":"ok"}`, `{"summary":"ok","blocks":["no","no"],"risks":[],"focus":[]}`, `{"summary":"ok","blocks":[""],"risks":[],"focus":[]}`} {
		if _, err := parseDailyPlanText(body); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestSourceSummaryAndSynthesisIsolation(t *testing.T) {
	var requests []string
	p := OpenAI{APIKey: "ollama", Model: "qwen3:4b", Client: &http.Client{Transport: diagnosticTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, string(body))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"message":{"content":"{\"summary\":\"source digest\",\"blocks\":[],\"risks\":[],\"focus\":[]}"}}`)), Header: make(http.Header)}, nil
	})}}
	summary, trace := p.SummarizeSource(context.Background(), "mail", model.SourceData{Mail: []model.MailMessage{{Subject: "raw-mail-secret"}}}, time.Now())
	if summary.Status != "ok" || trace.Source != "mail" {
		t.Fatalf("summary=%+v trace=%+v", summary, trace)
	}
	_, _, err := p.GenerateTrace(context.Background(), model.AppState{Mail: []model.MailMessage{{Subject: "raw-mail-secret"}}, SourceSummaries: []model.SourceSummary{summary}})
	if err != nil || len(requests) != 2 || !strings.Contains(requests[0], "raw-mail-secret") || strings.Contains(requests[1], "raw-mail-secret") || !strings.Contains(requests[1], "source digest") {
		t.Fatalf("unexpected requests: %v err=%v", requests, err)
	}
}

func TestSynthesisWithoutSuccessfulSummariesSkipsModel(t *testing.T) {
	p := OpenAI{APIKey: "ollama", Model: "qwen3:4b", Client: &http.Client{Transport: diagnosticTransport(func(r *http.Request) (*http.Response, error) { t.Fatal("model must not be called"); return nil, nil })}}
	_, trace, _ := p.GenerateTrace(context.Background(), model.AppState{SourceSummaries: []model.SourceSummary{{Name: "mail", Status: "failed"}}})
	if trace.Status != "failed" || trace.Stage != "source_summaries" {
		t.Fatalf("unexpected trace: %+v", trace)
	}
}
