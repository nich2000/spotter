package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"spotter/internal/model"
)

type SourceSummarizer interface {
	SummarizeSource(context.Context, string, model.SourceData, time.Time) (model.SourceSummary, Trace)
}

type Planner interface {
	Generate(ctx context.Context, input model.AppState) (model.DailyPlan, error)
}

type Trace struct {
	Source       string          `json:"source,omitempty"`
	Endpoint     string          `json:"endpoint,omitempty"`
	TimeoutMS    int64           `json:"timeoutMs,omitempty"`
	Request      json.RawMessage `json:"request,omitempty"`
	ResponseBody string          `json:"responseBody,omitempty"`

	OperationID     string `json:"operationId,omitempty"`
	Backend         string `json:"backend,omitempty"`
	Status          string `json:"status,omitempty"`
	Stage           string `json:"stage,omitempty"`
	DurationMS      int64  `json:"durationMs"`
	HTTPStatus      int    `json:"httpStatus,omitempty"`
	DoneReason      string `json:"doneReason,omitempty"`
	PromptTokens    int    `json:"promptTokens,omitempty"`
	OutputTokens    int    `json:"outputTokens,omitempty"`
	TotalDurationNS int64  `json:"totalDurationNs,omitempty"`
	LoadDurationNS  int64  `json:"loadDurationNs,omitempty"`
	EvalDurationNS  int64  `json:"evalDurationNs,omitempty"`

	Model    string `json:"model,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

type TracingPlanner interface {
	GenerateTrace(ctx context.Context, input model.AppState) (model.DailyPlan, Trace, error)
}

func GenerateWithTrace(ctx context.Context, planner Planner, input model.AppState) (model.DailyPlan, Trace, error) {
	if tracing, ok := planner.(TracingPlanner); ok {
		return tracing.GenerateTrace(ctx, input)
	}
	plan, err := planner.Generate(ctx, input)
	trace := Trace{OperationID: OperationID(ctx), Backend: fmt.Sprintf("%T", planner), Status: "ok"}
	if err != nil {
		trace.Status = "failed"
		trace.Error = err.Error()
	}
	return plan, trace, err
}

type RuleBased struct {
	Now func() time.Time
}

func (p RuleBased) Generate(ctx context.Context, input model.AppState) (model.DailyPlan, error) {
	select {
	case <-ctx.Done():
		return model.DailyPlan{}, ctx.Err()
	default:
	}

	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}

	unread := 0
	for _, msg := range input.Mail {
		if msg.IsUnread {
			unread++
		}
	}

	dueToday, overdue := splitReminders(input.Reminders, now)
	plan := model.DailyPlan{
		Summary: fmt.Sprintf("Сегодня %d событий, %d активных задач и %d непрочитанных писем.", len(input.Calendar), len(input.Reminders), unread),
		Blocks:  buildBlocks(input.Calendar, unread),
		Risks:   buildRisks(overdue, unread),
		Focus:   buildFocus(input.Calendar, dueToday, input.Notes),
	}
	if len(plan.Focus) == 0 {
		plan.Focus = append(plan.Focus, "Сохранить фокус на ключевых задачах дня")
	}
	return plan, nil
}

func buildBlocks(events []model.CalendarEvent, unread int) []string {
	if len(events) == 0 && unread == 0 {
		return []string{"Свободные блоки дня можно использовать для приоритетных задач"}
	}

	sorted := append([]model.CalendarEvent(nil), events...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start.Before(sorted[j].Start)
	})

	var blocks []string
	for _, event := range sorted {
		blocks = append(blocks, fmt.Sprintf("%s-%s - %s", event.Start.Format("15:04"), event.End.Format("15:04"), event.Title))
	}
	if unread > 0 {
		blocks = append(blocks, "Выделить время на разбор непрочитанных писем")
	}
	return blocks
}

func buildRisks(overdue []model.Reminder, unread int) []string {
	var risks []string
	if len(overdue) > 0 {
		risks = append(risks, "Есть просроченные напоминания")
	}
	if unread >= 10 {
		risks = append(risks, "Много непрочитанных писем")
	}
	if len(risks) == 0 {
		risks = append(risks, "Критичных рисков по текущим данным нет")
	}
	return risks
}

func buildFocus(events []model.CalendarEvent, dueToday []model.Reminder, notes []model.Note) []string {
	var focus []string
	if len(dueToday) > 0 {
		focus = append(focus, "Закрыть задачи с дедлайном сегодня")
	}
	if len(events) > 0 {
		focus = append(focus, "Подготовиться к ближайшей встрече")
	}
	if len(notes) > 0 {
		focus = append(focus, "Просмотреть заметки из папки Notes")
	}
	return focus
}

func splitReminders(reminders []model.Reminder, now time.Time) ([]model.Reminder, []model.Reminder) {
	var dueToday []model.Reminder
	var overdue []model.Reminder
	y, m, d := now.Date()
	todayStart := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.Add(24 * time.Hour)

	for _, reminder := range reminders {
		if reminder.DueDate == nil {
			continue
		}
		due := reminder.DueDate.In(now.Location())
		if due.Before(todayStart) {
			overdue = append(overdue, reminder)
		}
		if !due.Before(todayStart) && due.Before(tomorrowStart) {
			dueToday = append(dueToday, reminder)
		}
	}
	return dueToday, overdue
}
