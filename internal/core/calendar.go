package core

import (
	"fmt"
	"math"
	"sort"
	"time"
)

func validateCalendarSettings(v Object) error {
	for _, k := range []string{"pomodoroMinutes", "pomodoroBreakMinutes", "pomodoroLongBreakMinutes"} {
		if v[k] != nil && (number(v, k) < 1 || number(v, k) > 120 || math.Trunc(number(v, k)) != number(v, k)) {
			return invalid("Неверная длительность Pomodoro")
		}
	}
	for _, key := range []string{"workWeek", "workExceptions"} {
		if v[key] == nil {
			continue
		}
		m, ok := v[key].(map[string]any)
		if !ok {
			return invalid("Неверное рабочее расписание")
		}
		for date, raw := range m {
			if key == "workExceptions" && !validDay(date, str(v, "zone")) {
				return invalid("Неверная дата исключения")
			}
			if key == "workWeek" && !enum(date, "0", "1", "2", "3", "4", "5", "6") {
				return invalid("Неверный день недели")
			}
			if raw == nil {
				continue
			}
			w, ok := raw.(map[string]any)
			if !ok {
				return invalid("Неверное рабочее окно")
			}
			a, e := time.Parse("15:04", str(w, "start"))
			b, e2 := time.Parse("15:04", str(w, "end"))
			if e != nil || e2 != nil || !b.After(a) {
				return invalid("Неверное рабочее окно")
			}
		}
	}
	return nil
}
func workWindow(s State, date string, loc *time.Location) (time.Time, time.Time, error) {
	d, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return d, d, invalid("Неверная дата")
	}
	w := Object{"start": s.Settings["workStart"], "end": s.Settings["workEnd"]}
	if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		w = nil
	}
	if week := obj(s.Settings, "workWeek"); week != nil {
		if v, ok := week[fmt.Sprint(int(d.Weekday()))]; ok {
			w, _ = v.(map[string]any)
		}
	} else if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		w = nil
	}
	if exceptions := obj(s.Settings, "workExceptions"); exceptions != nil {
		if v, ok := exceptions[date]; ok {
			w, _ = v.(map[string]any)
		}
	}
	if w == nil {
		return d, d, invalid("Нерабочий день: " + date)
	}
	a, e := time.ParseInLocation("2006-01-02 15:04", date+" "+str(w, "start"), loc)
	b, e2 := time.ParseInLocation("2006-01-02 15:04", date+" "+str(w, "end"), loc)
	if e != nil || e2 != nil || !b.After(a) {
		return d, d, invalid("Неверное рабочее расписание")
	}
	return a, b, nil
}

type busySpan struct{ a, b time.Time }

func at(v Object, key string) time.Time  { t, _ := time.Parse(time.RFC3339, str(v, key)); return t }
func overlaps(a, b, c, d time.Time) bool { return a.Before(d) && b.After(c) }
func occupied(s State, now, a, b time.Time, taskID string) ([]busySpan, error) {
	spans := []busySpan{}
	for _, source := range s.CalendarSources {
		if !flag(source, "ok") || now.Sub(at(source, "observedAt")) > 24*time.Hour || at(source, "from").After(a) || at(source, "to").Before(b) {
			return nil, Fail("CALENDAR_STALE", 409, "Нет свежего покрытия внешнего календаря на выбранный день. Обновите helper.")
		}
		for _, raw := range list(source, "events") {
			e, _ := raw.(map[string]any)
			if flag(e, "cancelled") || str(e, "availability") == "free" || flag(e, "allDay") && str(e, "availability") != "busy" {
				continue
			}
			x, y := at(e, "startAt"), at(e, "endAt")
			if overlaps(a, b, x, y) {
				spans = append(spans, busySpan{x, y})
			}
		}
	}
	for id, t := range s.Tasks {
		if id == taskID {
			continue
		}
		v := obj(t, "scheduled")
		if v != nil && str(v, "kind") != "date_range" && v["endAt"] != nil {
			spans = append(spans, busySpan{at(v, "startAt"), at(v, "endAt")})
		}
	}
	return spans, nil
}
func unionMinutes(spans []busySpan, a, b time.Time) float64 {
	clipped := []busySpan{}
	for _, s := range spans {
		if s.a.Before(a) {
			s.a = a
		}
		if s.b.After(b) {
			s.b = b
		}
		if s.b.After(s.a) {
			clipped = append(clipped, s)
		}
	}
	sort.Slice(clipped, func(i, j int) bool { return clipped[i].a.Before(clipped[j].a) })
	total := 0.0
	end := a
	for _, s := range clipped {
		if s.a.Before(end) {
			s.a = end
		}
		if s.b.After(s.a) {
			total += s.b.Sub(s.a).Minutes()
			end = s.b
		}
	}
	return total
}
func blockAllowed(t Object, a, b time.Time) error {
	if v := obj(t, "scheduled"); v != nil {
		loc, _ := time.LoadLocation(str(v, "zone"))
		if str(v, "kind") == "date_range" {
			if a.In(loc).Format("2006-01-02") < str(v, "startDate") || str(v, "endDate") != "" && b.Add(-time.Nanosecond).In(loc).Format("2006-01-02") > str(v, "endDate") {
				return invalid("Рабочий блок вне интервала задачи")
			}
		} else if a.Before(at(v, "startAt")) || v["endAt"] != nil && b.After(at(v, "endAt")) {
			return invalid("Рабочий блок вне интервала задачи")
		}
	}
	if v := obj(t, "deadline"); v != nil {
		loc, _ := time.LoadLocation(str(v, "zone"))
		if str(v, "kind") == "date" {
			if b.Add(-time.Nanosecond).In(loc).Format("2006-01-02") > str(v, "date") {
				return invalid("Рабочий блок после дедлайна")
			}
		} else if b.After(at(v, "at")) {
			return invalid("Рабочий блок после дедлайна")
		}
	}
	return nil
}

// ProposeBlocks is pure. Confirm invokes it again under the workspace transaction lock.
func ProposeBlocks(s State, p Object, now time.Time) ([]Object, error) {
	if p["taskIds"] != nil {
		return distributeBlocks(s, p, now)
	}
	if err := checkKeys(p, "taskId", "startAt", "minutes", "zone", "pinned", "automatic"); err != nil {
		return nil, err
	}
	t, err := requiredEntity(s.Tasks, str(p, "taskId"))
	if err != nil {
		return nil, err
	}
	if str(t, "lifecycleState") == "done" {
		return nil, invalid("Задача завершена")
	}
	loc, err := time.LoadLocation(str(p, "zone"))
	if err != nil || str(p, "zone") != str(s.Settings, "zone") {
		return nil, invalid("Используйте часовой пояс рабочего расписания")
	}
	start, err := time.Parse(time.RFC3339, str(p, "startAt"))
	if err != nil {
		return nil, invalid("Неверное время начала")
	}
	n := number(p, "minutes")
	if n < 1 || n > 1440 || math.Trunc(n) != n {
		return nil, invalid("Объём должен быть целым числом минут от 1 до 1440")
	}
	remaining := t["remainingMinutes"]
	if remaining == nil {
		remaining = t["estimateMinutes"]
	}
	if remaining == nil {
		return nil, invalid("Сначала оцените задачу")
	}
	left := number(Object{"n": remaining}, "n")
	for _, b := range s.WorkBlocks {
		if str(b, "taskId") == str(p, "taskId") && str(b, "kind") == "work" && at(b, "endAt").After(now) {
			left -= at(b, "endAt").Sub(at(b, "startAt")).Minutes()
		}
	}
	if n > left {
		return nil, invalid(fmt.Sprintf("Недостаточно остатка: доступно %.0f мин", math.Max(0, left)))
	}
	a, b, err := workWindow(s, start.In(loc).Format("2006-01-02"), loc)
	if err != nil {
		return nil, err
	}
	busy, err := occupied(s, now, a, b, str(p, "taskId"))
	if err != nil {
		return nil, err
	}
	// With no connected source, manual placement is allowed; automatic availability is unknown.
	if flag(p, "automatic") && len(s.CalendarSources) == 0 {
		return nil, Fail("CALENDAR_STALE", 409, "Для автоматического подбора нужен свежий календарь")
	}
	chunk := number(s.Settings, "pomodoroMinutes")
	if chunk == 0 {
		chunk = 25
	}
	pause := number(s.Settings, "pomodoroBreakMinutes")
	if pause == 0 {
		pause = 5
	}
	long := number(s.Settings, "pomodoroLongBreakMinutes")
	if long == 0 {
		long = 15
	}
	if t["splittable"] == false {
		chunk = n
	}
	build := func(begin time.Time) []Object {
		out := []Object{}
		cursor := begin
		for rest, i := n, 0; rest > 0; i++ {
			size := math.Min(rest, chunk)
			end := cursor.Add(time.Duration(size) * time.Minute)
			out = append(out, Object{"taskId": p["taskId"], "kind": "work", "startAt": cursor.UTC().Format(time.RFC3339), "endAt": end.UTC().Format(time.RFC3339), "zone": p["zone"], "pinned": flag(p, "pinned")})
			rest -= size
			cursor = end
			if rest > 0 {
				gap := pause
				if (i+1)%4 == 0 {
					gap = long
				}
				cursor = cursor.Add(time.Duration(gap) * time.Minute)
				out = append(out, Object{"kind": "break", "startAt": end.UTC().Format(time.RFC3339), "endAt": cursor.UTC().Format(time.RFC3339), "zone": p["zone"], "pinned": flag(p, "pinned")})
			}
		}
		return out
	}
	used := []busySpan{}
	for _, v := range s.WorkBlocks {
		used = append(used, busySpan{at(v, "startAt"), at(v, "endAt")})
	}
	check := func(rows []Object) error {
		end := at(rows[len(rows)-1], "endAt")
		begin := at(rows[0], "startAt")
		if begin.Before(a) || end.After(b) {
			return invalid("Блоки не помещаются в рабочие часы")
		}
		if err := blockAllowed(t, begin, end); err != nil {
			return err
		}
		for _, v := range append(append([]busySpan{}, busy...), used...) {
			if overlaps(begin, end, v.a, v.b) {
				return invalid("Это время занято. Измените начало или используйте подбор")
			}
		}
		// Reserve is 20% after fixed events. Breaks consume the remaining budget.
		budget := (b.Sub(a).Minutes() - unionMinutes(busy, a, b)) * .8
		if unionMinutes(used, a, b)+end.Sub(begin).Minutes() > budget {
			return invalid("Недостаточно времени с учётом резерва 20% и перерывов")
		}
		return nil
	}
	if flag(p, "automatic") {
		if start.Before(a) {
			start = a
		}
		for cursor := start; cursor.Before(b); cursor = cursor.Add(5 * time.Minute) {
			rows := build(cursor)
			if check(rows) == nil {
				return rows, nil
			}
		}
		return nil, invalid(fmt.Sprintf("Не удалось разместить %.0f мин до конца дня/дедлайна. Измените объём, день или расписание", n))
	}
	rows := build(start)
	if err := check(rows); err != nil {
		return nil, err
	}
	return rows, nil
}
func (s *State) workBlockCommand(c Command, now time.Time) (Object, error) {
	p := c.Payload
	if s.WorkBlocks == nil {
		s.WorkBlocks = map[string]Object{}
	}
	switch c.Command {
	case "workblocks.create":
		rows, err := ProposeBlocks(*s, p, now)
		if err != nil {
			return nil, err
		}
		ids := []string{}
		for _, row := range rows {
			id := ID()
			row["id"] = id
			row["version"] = 1
			row["operationId"] = c.OperationID
			s.WorkBlocks[id] = row
			ids = append(ids, id)
			s.event(c.Command, id, c.OperationID, nil, row, now)
		}
		return Object{"blockIds": ids}, nil
	case "workblocks.delete", "workblocks.pin":
		if err := checkKeys(p, "blockId", "pinned"); err != nil {
			return nil, err
		}
		id := str(p, "blockId")
		row, err := requiredEntity(s.WorkBlocks, id)
		if err != nil {
			return nil, err
		}
		before := Copy(row)
		if c.Command == "workblocks.delete" {
			delete(s.WorkBlocks, id)
			s.event(c.Command, id, c.OperationID, before, nil, now)
		} else {
			if _, ok := p["pinned"].(bool); !ok {
				return nil, invalid("Нужен признак закрепления")
			}
			row["pinned"] = p["pinned"]
			row["version"] = number(row, "version") + 1
			s.event(c.Command, id, c.OperationID, before, row, now)
		}
		return Object{"blockId": id}, nil
	default:
		return nil, Fail("UNKNOWN_COMMAND", 400, "Неизвестная команда календаря")
	}
}

// Distribute only selected tasks, in deadline/importance order, without moving existing blocks.
func distributeBlocks(original State, p Object, now time.Time) ([]Object, error) {
	if err := checkKeys(p, "taskIds", "fromDate", "toDate", "zone"); err != nil {
		return nil, err
	}
	zone := str(p, "zone")
	loc, err := time.LoadLocation(zone)
	if err != nil || zone != str(original.Settings, "zone") {
		return nil, invalid("Неверный пояс")
	}
	from, e := time.Parse("2006-01-02", str(p, "fromDate"))
	to, e2 := time.Parse("2006-01-02", str(p, "toDate"))
	if e != nil || e2 != nil || to.Before(from) || to.Sub(from) > 30*24*time.Hour {
		return nil, invalid("Выберите период до 31 дня")
	}
	ids := list(p, "taskIds")
	if len(ids) == 0 || len(ids) > 50 {
		return nil, invalid("Выберите от 1 до 50 задач")
	}
	tasks := []Object{}
	seen := map[string]bool{}
	for _, v := range ids {
		id, ok := v.(string)
		if !ok || seen[id] {
			return nil, invalid("Неверный список задач")
		}
		t, err := requiredEntity(original.Tasks, id)
		if err != nil {
			return nil, err
		}
		seen[id] = true
		tasks = append(tasks, t)
	}
	due := func(t Object) string {
		d := obj(t, "deadline")
		if d == nil {
			return "9999"
		}
		if str(d, "kind") == "date" {
			return str(d, "date")
		}
		return str(d, "at")
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := due(tasks[i]), due(tasks[j])
		if a != b {
			return a < b
		}
		return str(tasks[i], "importance") == "yes" && str(tasks[j], "importance") != "yes"
	})
	s := Copy(original)
	if s.WorkBlocks == nil {
		s.WorkBlocks = map[string]Object{}
	}
	all := []Object{}
	for _, t := range tasks {
		remaining := t["remainingMinutes"]
		if remaining == nil {
			remaining = t["estimateMinutes"]
		}
		if remaining == nil {
			return nil, invalid("Сначала оцените задачу: " + str(t, "title"))
		}
		left := number(Object{"n": remaining}, "n")
		for _, b := range s.WorkBlocks {
			if str(b, "taskId") == str(t, "id") && str(b, "kind") == "work" && at(b, "endAt").After(now) {
				left -= at(b, "endAt").Sub(at(b, "startAt")).Minutes()
			}
		}
		for day := from; !day.After(to) && left > 0; day = day.AddDate(0, 0, 1) {
			a, _, e := workWindow(s, day.Format("2006-01-02"), loc)
			if e != nil {
				continue
			}
			size := left
			if size > 1440 {
				size = 1440
			}
			for size > 0 {
				rows, e := ProposeBlocks(s, Object{"taskId": t["id"], "startAt": a.Format(time.RFC3339), "zone": zone, "minutes": size, "automatic": true}, now)
				if e == nil {
					for _, row := range rows {
						s.WorkBlocks[ID()] = row
						all = append(all, row)
					}
					left -= size
					break
				}
				if ce, ok := e.(*Error); ok && ce.Code == "CALENDAR_STALE" {
					return nil, e
				}
				if t["splittable"] == false {
					break
				}
				size -= 25
				if size < 1 && size > -24 {
					size = 0
				}
			}
		}
		if left > 0 {
			return nil, invalid(fmt.Sprintf("Не хватает %.0f мин для %s. Измените объём, расписание или дедлайн", left, str(t, "title")))
		}
	}
	if len(all) == 0 {
		return nil, invalid("Нет нераспределённого объёма")
	}
	return all, nil
}
