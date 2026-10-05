package core

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

var Commands = []string{"workblocks.create", "workblocks.delete", "workblocks.pin", "source.accept", "source.dismiss", "source.restore", "task.create", "task.save", "task.triage", "task.transition", "task.reopen", "task.move", "project.create", "project.rename", "project.move", "plan.draft.save", "plan.draft.discard", "plan.activate", "plan.update", "plan.close", "settings.update", "wellbeing.save"}

// Apply changes a detached snapshot. The repository commits it with its receipt atomically.
func Apply(original State, c Command, now time.Time) (State, Receipt, error) {
	s := Copy(original)
	r := Receipt{APIVersion: "2", OperationID: c.OperationID, CommittedAt: now.UTC(), Warnings: []string{}, AffectedDomains: []string{"tasks", "inbox", "projects", "workday"}}
	if s.SchemaVersion != 2 {
		return original, r, Fail("SCHEMA_UNSUPPORTED", 503, "Неподдерживаемая схема")
	}
	if strings.TrimSpace(c.OperationID) == "" || utf8.RuneCountInString(c.OperationID) > 128 || c.ExpectedRevision < 0 {
		return original, r, invalid("Некорректный operationId/revision")
	}
	if c.ExpectedRevision != s.Revision {
		return original, r, Fail("VERSION_CONFLICT", 409, "Данные изменились. Черновик сохранён; перечитайте актуальную карточку.")
	}
	var result Object
	var err error
	switch {
	case strings.HasPrefix(c.Command, "workblocks."):
		result, err = s.workBlockCommand(c, now)
	case strings.HasPrefix(c.Command, "source."):
		result, err = s.candidateCommand(c, now)
	case strings.HasPrefix(c.Command, "task."):
		result, err = s.taskCommand(c, now)
	case strings.HasPrefix(c.Command, "project."):
		result, err = s.projectCommand(c, now)
	case strings.HasPrefix(c.Command, "plan."):
		result, err = s.planCommand(c, now)
	case c.Command == "settings.update":
		result, err = s.settingsCommand(c.Payload)
	case c.Command == "wellbeing.save":
		result, err = s.wellbeingCommand(c, now)
	case strings.HasPrefix(c.Command, "timer.") || strings.HasPrefix(c.Command, "focus.") || strings.HasPrefix(c.Command, "time."):
		err = policy("R3", "R9", "R11")
	case strings.HasPrefix(c.Command, "recurrence."):
		err = policy("R10", "R12")
	default:
		err = Fail("UNKNOWN_COMMAND", 400, "Неизвестная команда")
	}
	if err != nil {
		return original, r, err
	}
	if reflect.DeepEqual(s, Copy(original)) {
		result["noChange"] = true
	} else {
		if s.Revision >= 9007199254740991 {
			return original, r, invalid("Переполнение revision")
		}
		s.Revision++
	}
	if id := str(result, "taskId"); id != "" && deadlineExceeded(s.Tasks[id]) {
		r.Warnings = append(r.Warnings, "SCHEDULE_EXCEEDS_DEADLINE")
	}
	for _, block := range s.WorkBlocks {
		if task := s.Tasks[str(block, "taskId")]; task != nil && blockAllowed(task, at(block, "startAt"), at(block, "endAt")) != nil {
			r.Warnings = append(r.Warnings, "WORK_BLOCK_OUTSIDE_TASK_INTERVAL")
			break
		}
	}
	r.AffectedDomains = append(r.AffectedDomains, "planning", "calendar")
	r.Result = result
	r.CommittedRevision = s.Revision
	return s, r, nil
}
func text(v any, max int, required bool) error {
	t, ok := v.(string)
	if !ok || utf8.RuneCountInString(t) > max || (required && strings.TrimSpace(t) == "") {
		return invalid("Неверный текст или превышен размер поля")
	}
	return nil
}
func enum(v any, choices ...string) bool {
	for _, c := range choices {
		if v == c {
			return true
		}
	}
	return false
}
func (s *State) projectRef(v any) error {
	if v == nil {
		return nil
	}
	id, ok := v.(string)
	if !ok {
		return invalid("projectId должен быть строкой или null")
	}
	_, err := requiredEntity(s.Projects, id)
	return err
}
func (s *State) patch(t Object, p Object) error {
	if err := checkKeys(p, "title", "description", "resume", "projectId", "context", "importance", "urgency", "priorityNote", "complexity", "multiDay", "estimateMinutes", "remainingMinutes", "deadline", "scheduled", "inboxAvailableAt", "relatedTaskIds", "splittable"); err != nil {
		return err
	}
	for k, v := range p {
		switch k {
		case "splittable":
			if _, ok := v.(bool); !ok {
				return invalid("splittable должен быть boolean")
			}
		case "title":
			if err := text(v, 500, true); err != nil {
				return err
			}
		case "description":
			if err := text(v, 20000, false); err != nil {
				return err
			}
		case "resume", "priorityNote":
			if err := text(v, 5000, false); err != nil {
				return err
			}
		case "projectId":
			if err := s.projectRef(v); err != nil {
				return err
			}
		case "context":
			if !enum(v, "computer", "away", "anywhere") {
				return invalid("Некорректный контекст")
			}
		case "importance", "urgency":
			if !enum(v, "unknown", "yes", "no") {
				return invalid("Некорректный приоритет")
			}
		case "complexity":
			if v != nil && !enum(v, "easy", "medium", "hard") {
				return invalid("Некорректная сложность")
			}
		case "multiDay":
			if _, ok := v.(bool); !ok {
				return invalid("multiDay должен быть boolean")
			}
		case "estimateMinutes", "remainingMinutes":
			if v != nil {
				n, ok := v.(float64)
				min := float64(1)
				if k == "remainingMinutes" {
					min = 0
				}
				if !ok || math.Trunc(n) != n || n < min || n > 525600 {
					return invalid("Некорректная оценка времени")
				}
			}
		case "scheduled":
			if err := validateSchedule(v); err != nil {
				return err
			}
		case "deadline":
			if err := validateDeadline(v); err != nil {
				return err
			}
		case "relatedTaskIds":
			if err := s.validateLinks(str(t, "id"), v); err != nil {
				return err
			}
		case "inboxAvailableAt":
			if v != nil {
				return policy("R11", "R12")
			}
		}
		t[k] = v
	}
	return nil
}
func (s *State) taskCommand(c Command, now time.Time) (Object, error) {
	p := c.Payload
	if err := checkKeys(p, "title", "projectId", "patch", "taskId", "subtasks", "commentsToAdd", "triage", "transition", "reopen", "planChanges", "timeEntriesToAdd", "recurrenceChange", "completionEventId", "migrationBaselineId", "then"); err != nil {
		return nil, err
	}
	if obj(p, "recurrenceChange") != nil {
		return nil, policy("R10", "R12")
	}
	if len(list(p, "timeEntriesToAdd")) > 0 {
		return nil, policy("R11")
	}
	id := str(p, "taskId")
	var t Object
	var err error
	if c.Command == "task.create" {
		if err = text(p["title"], 500, true); err != nil {
			return nil, err
		}
		id = ID()
		t = Object{"id": id, "definitionId": ID(), "title": strings.TrimSpace(str(p, "title")), "description": "", "resume": "", "projectId": p["projectId"], "context": "anywhere", "importance": "unknown", "urgency": "unknown", "reviewRequired": true, "complexity": nil, "multiDay": false, "estimateMinutes": nil, "remainingMinutes": nil, "lifecycleState": "not_started", "workMode": nil, "completionEventId": nil, "createdAt": now.UTC().Format(time.RFC3339Nano), "subtasks": []any{}, "comments": []any{}}
		if err = s.projectRef(t["projectId"]); err != nil {
			return nil, err
		}
	} else {
		t, err = requiredEntity(s.Tasks, id)
		if err != nil {
			return nil, err
		}
	}
	before := Copy(t)
	refs := Object{}
	switch c.Command {
	case "task.create", "task.save":
		if patch := obj(p, "patch"); patch != nil {
			if c.Command == "task.create" {
				if v, ok := patch["title"]; ok && v != p["title"] {
					return nil, invalid("Конфликт title")
				}
				if v, ok := patch["projectId"]; ok && !reflect.DeepEqual(v, p["projectId"]) {
					return nil, invalid("Конфликт projectId")
				}
			}
			if err = s.patch(t, patch); err != nil {
				return nil, err
			}
		}
		if v, ok := p["subtasks"]; ok {
			rows, ok := v.([]any)
			if !ok || len(rows) > 200 {
				return nil, invalid("Некорректные подпункты")
			}
			old := map[string]bool{}
			for _, v := range list(t, "subtasks") {
				old[str(v.(map[string]any), "id")] = true
			}
			seen := map[string]bool{}
			out := []any{}
			for _, v := range rows {
				row, ok := v.(map[string]any)
				if !ok {
					return nil, invalid("Некорректный подпункт")
				}
				if err = checkKeys(row, "id", "clientRef", "title", "done", "position"); err != nil {
					return nil, err
				}
				if err = text(row["title"], 500, true); err != nil {
					return nil, err
				}
				if _, ok := row["done"].(bool); !ok {
					return nil, invalid("Требуется done")
				}
				rid := str(row, "id")
				if rid != "" {
					if !old[rid] || seen[rid] {
						return nil, invalid("Чужой или повторный подпункт")
					}
				} else {
					ref := str(row, "clientRef")
					if ref == "" || refs[ref] != nil {
						return nil, invalid("Требуется уникальный clientRef")
					}
					rid = ID()
					refs[ref] = rid
				}
				seen[rid] = true
				out = append(out, Object{"id": rid, "title": row["title"], "done": row["done"], "position": len(out)})
			}
			t["subtasks"] = out
		}
		if rows := list(p, "commentsToAdd"); len(rows) > 0 {
			if len(rows) > 50 {
				return nil, invalid("Слишком много комментариев")
			}
			for _, v := range rows {
				row, ok := v.(map[string]any)
				if !ok {
					return nil, invalid("Некорректный комментарий")
				}
				if err = checkKeys(row, "clientRef", "text"); err != nil {
					return nil, err
				}
				if err = text(row["text"], 5000, true); err != nil {
					return nil, err
				}
				ref := str(row, "clientRef")
				if ref == "" || refs[ref] != nil {
					return nil, invalid("Повторный clientRef")
				}
				cid := ID()
				refs[ref] = cid
				t["comments"] = append(list(t, "comments"), Object{"id": cid, "text": row["text"], "createdAt": now.UTC().Format(time.RFC3339Nano), "operationId": c.OperationID})
			}
		}
	case "task.move":
		if err = s.projectRef(p["projectId"]); err != nil {
			return nil, err
		}
		t["projectId"] = p["projectId"]
	case "task.triage", "task.transition", "task.reopen":
	default:
		return nil, Fail("UNKNOWN_COMMAND", 400, "Неизвестная команда задачи")
	}
	if triage := obj(p, "triage"); triage != nil {
		if err = checkKeys(triage, "action", "importance", "urgency", "priorityNote", "reason", "clearPriority"); err != nil {
			return nil, err
		}
		switch str(triage, "action") {
		case "complete":
			if !enum(triage["importance"], "yes", "no") || !enum(triage["urgency"], "yes", "no") {
				return nil, invalid("Укажите обе оси приоритета")
			}
			t["importance"] = triage["importance"]
			t["urgency"] = triage["urgency"]
			t["reviewRequired"] = false
		case "require":
			t["reviewRequired"] = true
			if flag(triage, "clearPriority") {
				t["importance"] = "unknown"
				t["urgency"] = "unknown"
			}
		default:
			return nil, invalid("Некорректный разбор")
		}
	}
	if tr := obj(p, "transition"); tr != nil {
		if obj(p, "reopen") != nil {
			return nil, invalid("Взаимоисключающие переходы")
		}
		if err = s.transition(t, tr, c, now); err != nil {
			return nil, err
		}
	}
	if c.Command == "task.reopen" || obj(p, "reopen") != nil {
		rp := p
		if obj(p, "reopen") != nil {
			rp = obj(p, "reopen")
		}
		if str(t, "lifecycleState") != "done" {
			return nil, invalid("Задача не завершена")
		}
		if t["completionEventId"] == nil {
			return nil, policy("R11")
		}
		if rp["completionEventId"] != t["completionEventId"] {
			return nil, Fail("VERSION_CONFLICT", 409, "Изменилось событие завершения")
		}
		t["lifecycleState"] = "not_started"
		t["workMode"] = nil
		t["completionEventId"] = nil
		t["reviewRequired"] = true
	}
	s.Tasks[id] = t
	if patch := obj(p, "patch"); patch != nil {
		if _, ok := patch["relatedTaskIds"]; ok {
			s.syncLinks(t, c, now)
		}
	}
	for _, v := range list(p, "planChanges") {
		pc, ok := v.(map[string]any)
		if !ok {
			return nil, invalid("Некорректное изменение плана")
		}
		if err = s.planChange(id, pc); err != nil {
			return nil, err
		}
	}
	if c.Command == "task.create" || !reflect.DeepEqual(before, t) {
		t["updatedAt"] = now.UTC().Format(time.RFC3339Nano)
		t["entityVersion"] = number(t, "entityVersion") + 1
		s.event(c.Command, id, c.OperationID, before, t, now)
	}
	return Object{"taskId": id, "definitionId": t["definitionId"], "clientRefs": refs}, nil
}
func (s *State) transition(t, tr Object, c Command, now time.Time) error {
	if err := checkKeys(tr, "to", "workMode", "waitingOn", "backgroundReason", "checkAt", "completionActor", "unfinishedSubtasksDecision"); err != nil {
		return err
	}
	to := str(tr, "to")
	if !enum(to, "not_started", "in_progress", "done") {
		return invalid("Некорректное состояние")
	}
	if str(t, "lifecycleState") == "done" {
		return invalid("Используйте повторное открытие")
	}
	mode := tr["workMode"]
	if to == "in_progress" {
		if !enum(mode, "active", "paused", "waiting", "background") {
			return invalid("Укажите режим работы")
		}
		if enum(mode, "waiting", "background") {
			if str(tr, "waitingOn") == "" && str(tr, "backgroundReason") == "" {
				return invalid("Укажите причину")
			}
			if tr["checkAt"] == nil {
				return invalid("Укажите контрольную дату")
			}
		}
	} else {
		mode = nil
	}
	if to == "done" {
		if !enum(tr["completionActor"], "self", "other") {
			return invalid("Укажите исполнителя")
		}
		for _, v := range list(t, "subtasks") {
			row := v.(map[string]any)
			if !flag(row, "done") {
				if !enum(tr["unfinishedSubtasksDecision"], "keep_open", "complete_all") {
					return Fail("UNFINISHED_SUBTASKS_DECISION_REQUIRED", 422, "Выберите действие с незавершёнными подпунктами")
				}
				if tr["unfinishedSubtasksDecision"] == "complete_all" {
					row["done"] = true
				}
			}
		}
		t["completionEventId"] = ID()
		t["completedAt"] = now.UTC().Format(time.RFC3339Nano)
		t["completionActor"] = tr["completionActor"]
	}
	t["lifecycleState"] = to
	t["workMode"] = mode
	t["waitingOn"] = tr["waitingOn"]
	t["backgroundReason"] = tr["backgroundReason"]
	t["checkAt"] = tr["checkAt"]
	return nil
}
func (s *State) projectCommand(c Command, now time.Time) (Object, error) {
	p := c.Payload
	if err := checkKeys(p, "projectId", "name", "parentId", "position"); err != nil {
		return nil, err
	}
	id := str(p, "projectId")
	var pr Object
	var err error
	if c.Command == "project.create" {
		id = ID()
		pr = Object{"id": id, "parentId": p["parentId"], "name": p["name"], "position": number(p, "position")}
	} else {
		pr, err = requiredEntity(s.Projects, id)
		if err != nil {
			return nil, err
		}
	}
	before := Copy(pr)
	switch c.Command {
	case "project.create":
	case "project.rename":
		pr["name"] = p["name"]
	case "project.move":
		pr["parentId"] = p["parentId"]
	default:
		return nil, Fail("UNKNOWN_COMMAND", 400, "Неизвестная команда проекта")
	}
	if err = text(pr["name"], 200, true); err != nil {
		return nil, err
	}
	pr["name"] = strings.TrimSpace(str(pr, "name"))
	if err = s.projectRef(pr["parentId"]); err != nil {
		return nil, err
	}
	for other, v := range s.Projects {
		if other != id && v["parentId"] == pr["parentId"] && v["name"] == pr["name"] {
			return nil, policy("R13")
		}
	}
	s.Projects[id] = pr
	for key := range s.Projects {
		seen := map[string]bool{}
		cur := key
		for depth := 0; cur != ""; depth++ {
			if seen[cur] {
				return nil, Fail("PROJECT_CYCLE", 422, "Циклическая иерархия")
			}
			if depth >= 32 {
				return nil, invalid("Глубина дерева больше 32")
			}
			seen[cur] = true
			cur = str(s.Projects[cur], "parentId")
		}
	}
	if c.Command == "project.create" || !reflect.DeepEqual(before, pr) {
		s.event(c.Command, id, c.OperationID, before, pr, now)
	}
	return Object{"projectId": id}, nil
}
func (s *State) settingsCommand(p Object) (Object, error) {
	if err := checkKeys(p, "patch"); err != nil {
		return nil, err
	}
	patch := obj(p, "patch")
	if err := checkKeys(patch, "zone", "workStart", "workEnd", "reservePercent", "focusBlockMinutes", "scheduleBreakMinutes", "plannerSleepTargetHours", "workWeek", "workExceptions", "pomodoroMinutes", "pomodoroBreakMinutes", "pomodoroLongBreakMinutes"); err != nil {
		return nil, err
	}
	v := Copy(s.Settings)
	for k, x := range patch {
		v[k] = x
	}
	if _, err := time.LoadLocation(str(v, "zone")); err != nil {
		return nil, invalid("Неверный часовой пояс")
	}
	a, e := time.Parse("15:04", str(v, "workStart"))
	b, e2 := time.Parse("15:04", str(v, "workEnd"))
	if e != nil || e2 != nil || !b.After(a) {
		return nil, invalid("Неверное рабочее окно")
	}
	for k, bounds := range map[string][2]float64{"reservePercent": {20, 30}, "focusBlockMinutes": {30, 120}, "scheduleBreakMinutes": {10, 20}, "plannerSleepTargetHours": {4, 12}} {
		n := number(v, k)
		if n < bounds[0] || n > bounds[1] {
			return nil, invalid(fmt.Sprintf("Поле %s вне диапазона", k))
		}
	}
	if err := validateCalendarSettings(v); err != nil {
		return nil, err
	}
	s.Settings = v
	return Object{"settingsVersion": s.Revision + 1}, nil
}
