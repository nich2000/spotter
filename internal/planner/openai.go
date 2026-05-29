package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"spotter/internal/model"
)

type OpenAI struct {
	APIKey   string
	Model    string
	Timeout  time.Duration
	Fallback Planner
	Client   *http.Client
}

func (p OpenAI) Generate(ctx context.Context, input model.AppState) (model.DailyPlan, error) {
	fallback := p.Fallback
	if fallback == nil {
		fallback = RuleBased{}
	}

	base, err := fallback.Generate(ctx, input)
	if err != nil {
		return model.DailyPlan{}, err
	}
	if p.APIKey == "" {
		return base, nil
	}

	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	plan, err := p.generateWithOpenAI(callCtx, input, base)
	if err != nil {
		base.Risks = append(base.Risks, "OpenAI рекомендации недоступны: "+err.Error())
		return base, nil
	}
	return plan, nil
}

func (p OpenAI) generateWithOpenAI(ctx context.Context, input model.AppState, base model.DailyPlan) (model.DailyPlan, error) {
	modelName := p.Model
	if modelName == "" {
		modelName = "gpt-5.2"
	}

	payload := map[string]any{
		"model": modelName,
		"instructions": strings.TrimSpace(`Ты персональный ассистент. На основе локального контекста дня сформируй короткие практические рекомендации на русском.
Ответ верни строго JSON-объектом:
{
  "summary": "одно предложение",
  "blocks": ["временные блоки или действия"],
  "risks": ["риски"],
  "focus": ["фокус и рекомендации"]
}
Не добавляй markdown. Не придумывай факты, которых нет в контексте.`),
		"input":             buildPlannerContext(input, base),
		"max_output_tokens": 800,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return model.DailyPlan{}, fmt.Errorf("marshal request: %w", err)
	}

	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(raw))
	if err != nil {
		return model.DailyPlan{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return model.DailyPlan{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var decoded responsePayload
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return model.DailyPlan{}, fmt.Errorf("parse response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decoded.Error.Message != "" {
			return model.DailyPlan{}, errors.New(decoded.Error.Message)
		}
		return model.DailyPlan{}, fmt.Errorf("http status %d", resp.StatusCode)
	}

	text := decoded.OutputText
	if text == "" {
		text = decoded.textFromOutput()
	}
	if text == "" {
		return model.DailyPlan{}, fmt.Errorf("empty response text")
	}

	var plan model.DailyPlan
	if err := json.Unmarshal([]byte(text), &plan); err != nil {
		return model.DailyPlan{}, fmt.Errorf("parse plan json: %w", err)
	}
	if plan.Summary == "" {
		plan.Summary = base.Summary
	}
	if len(plan.Blocks) == 0 {
		plan.Blocks = base.Blocks
	}
	if len(plan.Risks) == 0 {
		plan.Risks = base.Risks
	}
	if len(plan.Focus) == 0 {
		plan.Focus = base.Focus
	}
	return plan, nil
}

func buildPlannerContext(input model.AppState, base model.DailyPlan) string {
	type contextState struct {
		GeneratedAt time.Time             `json:"generatedAt"`
		Calendar    []model.CalendarEvent `json:"calendar"`
		Reminders   []model.Reminder      `json:"reminders"`
		UnreadMail  []model.MailMessage   `json:"unreadMail"`
		Notes       []model.Note          `json:"notes"`
		Sources     []model.SourceStatus  `json:"sources"`
		BasePlan    model.DailyPlan       `json:"basePlan"`
	}
	raw, _ := json.Marshal(contextState{
		GeneratedAt: input.GeneratedAt,
		Calendar:    input.Calendar,
		Reminders:   input.Reminders,
		UnreadMail:  input.Mail,
		Notes:       input.Notes,
		Sources:     input.Sources,
		BasePlan:    base,
	})
	return string(raw)
}

type responsePayload struct {
	OutputText string `json:"output_text"`
	Output     []struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (r responsePayload) textFromOutput() string {
	for _, item := range r.Output {
		for _, content := range item.Content {
			if content.Text != "" {
				return content.Text
			}
		}
	}
	return ""
}
