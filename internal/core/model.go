// Package core implements transport-independent workspace commands.
package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type Object = map[string]any

type State struct {
	WorkBlocks       map[string]Object `json:"workBlocks"`
	CalendarSources  map[string]Object `json:"calendarSources"`
	SourceSnapshotID string            `json:"sourceSnapshotId"`
	Candidates       map[string]Object `json:"candidates"`
	SchemaVersion    int               `json:"schemaVersion"`
	Revision         int64             `json:"revision"`
	Tasks            map[string]Object `json:"tasks"`
	Projects         map[string]Object `json:"projects"`
	Plans            map[string]Object `json:"plans"`
	Snapshots        map[string]Object `json:"snapshots"`
	Days             map[string]Object `json:"days"`
	Events           []Object          `json:"events"`
	Focus            *string           `json:"focusOccurrenceId"`
	Settings         Object            `json:"settings"`
	Migration        Object            `json:"migration"`
}

func NewState() State {
	return State{SchemaVersion: 2, Candidates: map[string]Object{}, Tasks: map[string]Object{}, Projects: map[string]Object{}, Plans: map[string]Object{}, Snapshots: map[string]Object{}, Days: map[string]Object{}, Events: []Object{}, Settings: Object{"zone": "Europe/Moscow", "workStart": "09:30", "workEnd": "18:00", "reservePercent": 25, "focusBlockMinutes": 90, "scheduleBreakMinutes": 15, "plannerSleepTargetHours": 8}, Migration: Object{"status": "new", "historyCompleteness": "unknown"}}
}
func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Copy[T any](v T) T { b, _ := json.Marshal(v); var out T; _ = json.Unmarshal(b, &out); return out }

type Command struct {
	OperationID      string `json:"operationId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Command          string `json:"command"`
	Payload          Object `json:"payload"`
}
type Receipt struct {
	APIVersion        string    `json:"apiVersion"`
	OperationID       string    `json:"operationId"`
	CommittedRevision int64     `json:"committedRevision"`
	CommittedAt       time.Time `json:"committedAt"`
	Result            Object    `json:"result"`
	AffectedDomains   []string  `json:"affectedDomains"`
	Warnings          []string  `json:"warnings"`
}
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
	Details Object `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }
func Fail(code string, status int, message string) *Error {
	return &Error{Code: code, Status: status, Message: message}
}
func invalid(message string) error { return Fail("VALIDATION_FAILED", 422, message) }
func policy(ids ...string) error {
	return &Error{Code: "POLICY_UNRESOLVED", Status: 409, Message: "Требуется выбор продуктовой политики", Details: Object{"decisionIds": ids}}
}
func str(m Object, k string) string { v, _ := m[k].(string); return v }
func obj(m Object, k string) Object { v, _ := m[k].(map[string]any); return v }
func number(m Object, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0
}
func flag(m Object, k string) bool  { v, _ := m[k].(bool); return v }
func list(m Object, k string) []any { v, _ := m[k].([]any); return v }
func (s *State) event(kind, id, op string, before, after any, now time.Time) {
	s.Events = append(s.Events, Object{"id": ID(), "sequence": len(s.Events) + 1, "kind": kind, "entityId": id, "operationId": op, "occurredAt": now.UTC().Format(time.RFC3339Nano), "before": before, "after": Copy(after)})
}
func requiredEntity(set map[string]Object, id string) (Object, error) {
	v, ok := set[id]
	if !ok {
		return nil, Fail("ENTITY_NOT_FOUND", 404, "Объект не найден")
	}
	return v, nil
}
func checkKeys(m Object, keys ...string) error {
	allowed := map[string]bool{}
	for _, k := range keys {
		allowed[k] = true
	}
	for k := range m {
		if !allowed[k] {
			return Fail("UNKNOWN_FIELD", 400, fmt.Sprintf("Неизвестное поле: %s", k))
		}
	}
	return nil
}
