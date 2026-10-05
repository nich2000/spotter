package core

import (
	"reflect"
	"time"
)

func validDay(date, zone string) bool {
	_, e := time.Parse("2006-01-02", date)
	_, e2 := time.LoadLocation(zone)
	return e == nil && e2 == nil
}
func (s *State) planItems(p Object) error {
	rows, ok := p["items"].([]any)
	if !ok {
		return invalid("Нужен список задач плана")
	}
	if len(rows) > 3 {
		return policy("R4")
	}
	seen := map[string]bool{}
	for i, v := range rows {
		row, ok := v.(map[string]any)
		if !ok {
			return invalid("Некорректный элемент плана")
		}
		id := str(row, "taskId")
		if _, err := requiredEntity(s.Tasks, id); err != nil {
			return err
		}
		if seen[id] {
			return invalid("Задача повторяется в плане")
		}
		seen[id] = true
		if row["allocatedMinutes"] != nil {
			n := number(row, "allocatedMinutes")
			if n < 1 || n > 525600 {
				return invalid("Неверный дневной объём")
			}
		}
		row["position"] = i
	}
	for _, key := range []string{"mainOccurrenceId", "frogOccurrenceId"} {
		if p[key] != nil && !seen[str(p, key)] {
			return invalid("Выбранная задача отсутствует в плане")
		}
	}
	if id := str(p, "frogOccurrenceId"); id != "" && s.Tasks[id]["importance"] != "yes" {
		return invalid("Лягушка должна быть важной задачей")
	}
	return nil
}
func (s *State) planCommand(c Command, now time.Time) (Object, error) {
	p := c.Payload
	if err := checkKeys(p, "planId", "date", "zone", "basedOnPlanId", "items", "mainOccurrenceId", "frogOccurrenceId", "activate", "summaryText", "nextDraft"); err != nil {
		return nil, err
	}
	if flag(p, "activate") && c.Command != "plan.draft.save" && c.Command != "plan.activate" {
		return nil, invalid("Активация разрешена только при сохранении или активации черновика")
	}
	id := str(p, "planId")
	var plan Object
	var err error
	if c.Command == "plan.draft.save" && id == "" {
		date, zone := str(p, "date"), str(p, "zone")
		if !validDay(date, zone) {
			return nil, invalid("Неверная дата/зона")
		}
		loc, _ := time.LoadLocation(zone)
		if date != now.In(loc).Format("2006-01-02") {
			return nil, policy("R5")
		}
		version := float64(1)
		for _, v := range s.Plans {
			if v["date"] == date {
				if v["zone"] != zone {
					return nil, invalid("Часовой пояс даты неизменяем")
				}
				if n := number(v, "dayVersion") + 1; n > version {
					version = n
				}
			}
		}
		id = ID()
		plan = Object{"id": id, "date": date, "zone": zone, "dayVersion": version, "status": "draft", "createdAt": now.UTC().Format(time.RFC3339Nano)}
	} else {
		plan, err = requiredEntity(s.Plans, id)
		if err != nil {
			return nil, err
		}
	}
	before := Copy(plan)
	switch c.Command {
	case "plan.draft.save", "plan.update":
		want := "draft"
		if c.Command == "plan.update" {
			want = "active"
		}
		if plan["status"] != want {
			return nil, Fail("PLAN_NOT_EDITABLE", 409, "План нельзя редактировать")
		}
		if p["date"] != nil && p["date"] != plan["date"] || p["zone"] != nil && p["zone"] != plan["zone"] {
			return nil, invalid("Дата и пояс неизменяемы")
		}
		if err = s.planItems(p); err != nil {
			return nil, err
		}
		for _, k := range []string{"items", "mainOccurrenceId", "frogOccurrenceId"} {
			plan[k] = p[k]
		}
	case "plan.activate":
		if plan["status"] != "draft" {
			return nil, Fail("PLAN_NOT_EDITABLE", 409, "Нужен черновик")
		}
	case "plan.draft.discard":
		if plan["status"] != "draft" {
			return nil, Fail("PLAN_NOT_EDITABLE", 409, "Нужен черновик")
		}
		plan["status"] = "discarded"
	case "plan.close":
		return nil, policy("R5", "R9", "R11")
	default:
		return nil, Fail("UNKNOWN_COMMAND", 400, "Неизвестная команда плана")
	}
	if c.Command == "plan.activate" || flag(p, "activate") {
		for pid, v := range s.Plans {
			if pid != id && v["date"] == plan["date"] && v["status"] == "active" {
				return nil, Fail("ACTIVE_PLAN_EXISTS", 409, "У даты уже есть активный план")
			}
		}
		plan["status"] = "active"
		plan["activatedAt"] = now.UTC().Format(time.RFC3339Nano)
	}
	s.Plans[id] = plan
	if !reflect.DeepEqual(before, plan) {
		s.event(c.Command, id, c.OperationID, before, plan, now)
	}
	return Object{"planId": id, "dayVersion": plan["dayVersion"], "status": plan["status"]}, nil
}
func (s *State) planChange(taskID string, p Object) error {
	plan, err := requiredEntity(s.Plans, str(p, "planId"))
	if err != nil {
		return err
	}
	if plan["status"] != "active" {
		return Fail("PLAN_NOT_EDITABLE", 409, "Нужен активный план")
	}
	rows := []any{}
	for _, v := range list(plan, "items") {
		if str(v.(map[string]any), "taskId") != taskID {
			rows = append(rows, v)
		}
	}
	switch str(p, "action") {
	case "upsert":
		rows = append(rows, Object{"taskId": taskID, "allocatedMinutes": p["allocatedMinutes"], "position": len(rows)})
	case "remove":
		for _, k := range []string{"mainOccurrenceId", "frogOccurrenceId"} {
			if plan[k] == taskID {
				clear := "clearMain"
				if k == "frogOccurrenceId" {
					clear = "clearFrog"
				}
				if !flag(p, clear) {
					return invalid("Подтвердите снятие главной задачи/лягушки")
				}
				plan[k] = nil
			}
		}
	default:
		return invalid("Неверное изменение плана")
	}
	plan["items"] = rows
	return s.planItems(plan)
}
func (s *State) wellbeingCommand(c Command, now time.Time) (Object, error) {
	p := c.Payload
	if err := checkKeys(p, "date", "zone", "note", "wellbeing", "habits", "measurementsToAdd", "selectMeasurements"); err != nil {
		return nil, err
	}
	date, zone := str(p, "date"), str(p, "zone")
	if !validDay(date, zone) {
		return nil, invalid("Неверная дата/зона")
	}
	if len(list(p, "measurementsToAdd")) > 0 || obj(p, "selectMeasurements") != nil {
		return nil, policy("R7", "R11")
	}
	day := s.Days[date]
	if day == nil {
		day = Object{"id": ID(), "date": date, "zone": zone, "note": "", "wellbeing": nil, "habits": nil, "wellbeingConfirmedAt": nil, "habitsConfirmedAt": nil}
	}
	if day["zone"] != zone {
		return nil, Fail("FIELD_IMMUTABLE", 422, "Пояс записи неизменяем")
	}
	before := Copy(day)
	if note, ok := p["note"]; ok {
		if err := text(note, 20000, false); err != nil {
			return nil, err
		}
		day["note"] = note
	}
	if v := obj(p, "wellbeing"); v != nil {
		if err := checkKeys(v, "mood", "energy", "stress", "confirm"); err != nil {
			return nil, err
		}
		if !enum(v["mood"], "unknown", "good", "normal", "bad") || !enum(v["energy"], "unknown", "high", "medium", "low") || !enum(v["stress"], "unknown", "low", "medium", "high") {
			return nil, invalid("Неверная шкала состояния")
		}
		if _, ok := v["confirm"].(bool); !ok {
			return nil, invalid("Укажите подтверждение")
		}
		day["wellbeing"] = v
		day["wellbeingConfirmedAt"] = nil
		if flag(v, "confirm") {
			day["wellbeingConfirmedAt"] = now.UTC().Format(time.RFC3339Nano)
		}
	}
	if v := obj(p, "habits"); v != nil {
		if err := checkKeys(v, "cigarettes", "alcoholPortions", "confirm"); err != nil {
			return nil, err
		}
		for _, key := range []string{"cigarettes", "alcoholPortions"} {
			if v[key] != nil {
				n, ok := v[key].(float64)
				if !ok || n < 0 || n > 1000000 || n != float64(int(n)) {
					return nil, invalid("Неверный счётчик привычек")
				}
			}
		}
		if _, ok := v["confirm"].(bool); !ok {
			return nil, invalid("Укажите подтверждение")
		}
		day["habits"] = v
		day["habitsConfirmedAt"] = nil
		if flag(v, "confirm") {
			day["habitsConfirmedAt"] = now.UTC().Format(time.RFC3339Nano)
		}
	}
	s.Days[date] = day
	if !reflect.DeepEqual(before, day) {
		s.event(c.Command, str(day, "id"), c.OperationID, nil, Object{"date": date}, now)
	}
	return Object{"dayRecordId": day["id"], "date": date}, nil
}
