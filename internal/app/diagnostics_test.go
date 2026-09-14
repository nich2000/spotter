package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"spotter/internal/collectors"
	"strings"
	"sync"
	"testing"
	"time"

	"spotter/internal/audit"
	"spotter/internal/model"
	"spotter/internal/planner"
	"spotter/internal/sse"
)

type failedModel struct{}

func (failedModel) Generate(context.Context, model.AppState) (model.DailyPlan, error) {
	return model.DailyPlan{}, nil
}
func (failedModel) GenerateTrace(context.Context, model.AppState) (model.DailyPlan, planner.Trace, error) {
	return model.DailyPlan{Summary: "unavailable"}, planner.Trace{Status: "failed", Stage: "validate_plan", Error: "invalid plan"}, nil
}

type recordingAudit struct{ record audit.RefreshRecord }

func (s *recordingAudit) SaveRefresh(_ context.Context, r audit.RefreshRecord) error {
	s.record = r
	return nil
}
func TestRefreshReportsModelFailureAndCorrelatesAudit(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	store := &recordingAudit{}
	app := New(logger, nil, failedModel{}, &memoryStore{}, store, sse.NewBroker(logger))
	app.Refresh(planner.WithTrigger(context.Background(), "manual_http"))
	if store.record.OperationID == "" || store.record.Trigger != "manual_http" || store.record.Model.Error != "invalid plan" {
		t.Fatalf("audit: %+v", store.record)
	}
	for _, want := range []string{`"status":"degraded"`, `"failures":1`, store.record.OperationID, `"msg":"audit saved"`, `"msg":"state saved"`, `"msg":"state published"`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("missing %s in %s", want, logs.String())
		}
	}
}

type summaryCollector struct {
	name string
	fail bool
}

func (c summaryCollector) Name() string { return c.name }
func (c summaryCollector) Collect(context.Context) (model.SourceData, error) {
	if c.fail {
		return model.SourceData{}, fmt.Errorf("collection failed")
	}
	return model.SourceData{}, nil
}

type summaryPlanner struct {
	mu    sync.Mutex
	calls int
	seen  model.AppState
}

func (p *summaryPlanner) SummarizeSource(_ context.Context, name string, _ model.SourceData, _ time.Time) (model.SourceSummary, planner.Trace) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if name == "notes" {
		return model.SourceSummary{Name: name, Status: "failed", Error: "model failed"}, planner.Trace{Error: "model failed"}
	}
	return model.SourceSummary{Name: name, Status: "ok", Content: model.DailyPlan{Summary: "digest"}}, planner.Trace{Status: "ok", Response: "digest"}
}
func (p *summaryPlanner) Generate(_ context.Context, state model.AppState) (model.DailyPlan, error) {
	p.seen = state
	return model.DailyPlan{Summary: "day focus"}, nil
}
func TestRefreshSummarizesSuccessfulSourcesBeforeSynthesis(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auditStore := &recordingAudit{}
	p := &summaryPlanner{}
	a := New(logger, []collectors.Collector{summaryCollector{"calendar", true}, summaryCollector{"mail", false}, summaryCollector{"notes", false}}, p, &memoryStore{}, auditStore, sse.NewBroker(logger))
	state := a.Refresh(context.Background())
	if p.calls != 2 || len(p.seen.SourceSummaries) != 3 || state.Plan.Summary != "day focus" {
		t.Fatalf("calls=%d state=%+v", p.calls, state)
	}
	for i, want := range []string{"skipped", "ok", "failed"} {
		if state.SourceSummaries[i].Status != want {
			t.Fatalf("summary %d: %+v", i, state.SourceSummaries[i])
		}
	}
	if auditStore.record.Sources[1].Model.Response != "digest" || auditStore.record.Sources[2].Summary.Error != "model failed" {
		t.Fatalf("audit=%+v", auditStore.record)
	}
}
