package core

import (
	"sort"
	"time"
)

func validateSchedule(v any) error {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return invalid("scheduled должен быть объектом или null")
	}
	if _, err := time.LoadLocation(str(m, "zone")); err != nil || str(m, "zone") == "" {
		return invalid("Неверный часовой пояс")
	}
	switch str(m, "kind") {
	case "date_range":
		if err := checkKeys(m, "kind", "startDate", "endDate", "zone"); err != nil {
			return err
		}
		if !validDay(str(m, "startDate"), str(m, "zone")) {
			return invalid("Неверная дата начала")
		}
		if m["endDate"] != nil && (!validDay(str(m, "endDate"), str(m, "zone")) || str(m, "endDate") < str(m, "startDate")) {
			return invalid("Окончание раньше начала или неверная дата")
		}
	case "timed", "": // Preserve compatibility with Scheduled v2.
		if err := checkKeys(m, "kind", "startAt", "endAt", "zone", "localStart", "localEnd", "offsetChoice"); err != nil {
			return err
		}
		if m["offsetChoice"] != nil && !enum(m["offsetChoice"], "earlier", "later") {
			return invalid("Неверный offsetChoice")
		}
		start, err := time.Parse(time.RFC3339, str(m, "startAt"))
		if err != nil {
			return invalid("Неверное время начала")
		}
		if m["endAt"] != nil {
			end, e := time.Parse(time.RFC3339, str(m, "endAt"))
			if e != nil || !end.After(start) {
				return invalid("Конец должен быть позже начала")
			}
		}
		loc, _ := time.LoadLocation(str(m, "zone"))
		for _, pair := range [][2]string{{"startAt", "localStart"}, {"endAt", "localEnd"}} {
			if m[pair[1]] != nil {
				at, e := time.Parse(time.RFC3339, str(m, pair[0]))
				if e != nil || at.In(loc).Format("2006-01-02T15:04:05") != str(m, pair[1]) {
					return invalid("Местное время не соответствует часовому поясу")
				}
				if err := validateLocalChoice(at, loc, str(m, pair[1]), str(m, "offsetChoice")); err != nil {
					return err
				}
			}
		}
	default:
		return invalid("Неизвестный тип scheduled")
	}
	return nil
}
func validateDeadline(v any) error {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return invalid("deadline должен быть объектом или null")
	}
	if _, e := time.LoadLocation(str(m, "zone")); e != nil || str(m, "zone") == "" {
		return invalid("Неверный часовой пояс дедлайна")
	}
	switch str(m, "kind") {
	case "date":
		if e := checkKeys(m, "kind", "date", "zone"); e != nil {
			return e
		}
		if !validDay(str(m, "date"), str(m, "zone")) {
			return invalid("Неверная дата дедлайна")
		}
	case "instant":
		if e := checkKeys(m, "kind", "at", "zone"); e != nil {
			return e
		}
		if _, e := time.Parse(time.RFC3339, str(m, "at")); e != nil {
			return invalid("Неверное время дедлайна")
		}
	default:
		return invalid("Неизвестный тип дедлайна")
	}
	return nil
}
func deadlineExceeded(t Object) bool {
	s, d := obj(t, "scheduled"), obj(t, "deadline")
	if s == nil || d == nil {
		return false
	}
	var end time.Time
	sl, _ := time.LoadLocation(str(s, "zone"))
	dl, _ := time.LoadLocation(str(d, "zone"))
	if sl == nil || dl == nil {
		return false
	}
	if str(s, "kind") == "date_range" {
		date := str(s, "endDate")
		if date == "" {
			date = str(s, "startDate")
		}
		end, _ = time.ParseInLocation("2006-01-02", date, sl)
		end = end.AddDate(0, 0, 1)
	} else {
		v := str(s, "endAt")
		if v == "" {
			v = str(s, "startAt")
		}
		end, _ = time.Parse(time.RFC3339, v)
	}
	var limit time.Time
	if str(d, "kind") == "date" {
		limit, _ = time.ParseInLocation("2006-01-02", str(d, "date"), dl)
		limit = limit.AddDate(0, 0, 1)
	} else {
		limit, _ = time.Parse(time.RFC3339, str(d, "at"))
	}
	return end.After(limit)
}
func (s *State) validateLinks(taskID string, v any) error {
	rows, ok := v.([]any)
	if !ok {
		return invalid("Нужен список связей")
	}
	seen := map[string]bool{}
	for _, v := range rows {
		id, ok := v.(string)
		if !ok || id == taskID || seen[id] {
			return invalid("Некорректная или повторная связь")
		}
		if _, e := requiredEntity(s.Tasks, id); e != nil {
			return e
		}
		seen[id] = true
	}
	return nil
}

// Links are undirected; both endpoints are committed in the same workspace transaction.
func (s *State) syncLinks(t Object, c Command, now time.Time) {
	id := str(t, "id")
	want := map[string]bool{}
	for _, v := range list(t, "relatedTaskIds") {
		want[v.(string)] = true
	}
	for otherID, other := range s.Tasks {
		if otherID == id {
			continue
		}
		has := false
		rows := []any{}
		for _, v := range list(other, "relatedTaskIds") {
			if v == id {
				has = true
			} else {
				rows = append(rows, v)
			}
		}
		if has == want[otherID] {
			continue
		}
		before := Copy(other)
		if want[otherID] {
			rows = append(rows, id)
		}
		other["relatedTaskIds"] = rows
		other["entityVersion"] = number(other, "entityVersion") + 1
		other["updatedAt"] = now.UTC().Format(time.RFC3339Nano)
		s.event("task.links", otherID, c.OperationID, before, other, now)
	}
}

// Explicit local wall times require a fold choice; instant-only clients already identify an unambiguous instant.
func validateLocalChoice(at time.Time, loc *time.Location, local, choice string) error {
	base, err := time.Parse("2006-01-02T15:04:05", local)
	if err != nil {
		return invalid("Неверное местное время")
	}
	offsets := map[int]bool{}
	for h := -48; h <= 48; h += 6 {
		_, offset := base.Add(time.Duration(h) * time.Hour).In(loc).Zone()
		offsets[offset] = true
	}
	matches := []time.Time{}
	for offset := range offsets {
		candidate := base.Add(-time.Duration(offset) * time.Second)
		if candidate.In(loc).Format("2006-01-02T15:04:05") == local {
			matches = append(matches, candidate)
		}
	}
	if len(matches) > 1 {
		sort.Slice(matches, func(i, j int) bool { return matches[i].Before(matches[j]) })
		var chosen time.Time
		switch choice {
		case "earlier":
			chosen = matches[0]
		case "later":
			chosen = matches[len(matches)-1]
		default:
			return invalid("Неоднозначное местное время требует offsetChoice")
		}
		if !at.Truncate(time.Second).Equal(chosen) {
			return invalid("offsetChoice не соответствует Instant")
		}
	}
	return nil
}
