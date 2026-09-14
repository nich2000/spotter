// Package workspace owns user decisions independently from collected snapshots.
package workspace

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"spotter/internal/model"
)

type Task struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Project   string    `json:"project"`
	Status    string    `json:"status"`
	Minutes   int       `json:"minutes"`
	MITDate   string    `json:"mitDate,omitempty"`
	MainDate  string    `json:"mainDate,omitempty"`
	Resume    string    `json:"resume,omitempty"`
	WaitingOn string    `json:"waitingOn,omitempty"`
	CheckDate string    `json:"checkDate,omitempty"`
	Weekly    bool      `json:"weekly"`
	Source    string    `json:"source"`
	SourceKey string    `json:"sourceKey,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type Settings struct {
	Start       string  `json:"start"`
	End         string  `json:"end"`
	Zone        string  `json:"zone"`
	Reserve     int     `json:"reserve"`
	Block       int     `json:"block"`
	Break       int     `json:"break"`
	SleepTarget float64 `json:"sleepTarget"`
}
type Review struct {
	At      time.Time `json:"at"`
	Results string    `json:"results"`
}
type Data struct {
	Version    int      `json:"version"`
	Tasks      []Task   `json:"tasks"`
	Settings   Settings `json:"settings"`
	Health     *Health  `json:"health,omitempty"`
	Energy     string   `json:"energy"`
	EnergyDate string   `json:"energyDate"`
	Review     Review   `json:"review"`
	Dismissed  []string `json:"dismissed,omitempty"`
}
type Command struct {
	Version  int      `json:"version"`
	Action   string   `json:"action"`
	Task     Task     `json:"task"`
	Key      string   `json:"key"`
	Date     string   `json:"date"`
	Energy   string   `json:"energy"`
	Settings Settings `json:"settings"`
	Results  string   `json:"results"`
}
type Candidate struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Project string `json:"project"`
	Source  string `json:"source"`
	Count   int    `json:"count"`
	Details string `json:"details,omitempty"`
}
type Service struct {
	mu   sync.Mutex
	path string
	data Data
}

var ErrConflict = errors.New("Данные изменились в другой вкладке. Обновите рабочую панель и повторите действие.")

func New(path string) (*Service, error) {
	s := &Service{path: path, data: Data{Tasks: []Task{}, Settings: Settings{Start: "09:30", End: "18:00", Zone: "Europe/Moscow", Reserve: 25, Block: 90, Break: 15, SleepTarget: 8}}}
	raw, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(raw, &s.data)
	} else if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	if err = validateSettings(s.data.Settings); err != nil {
		return nil, err
	}
	return s, nil
}
func clone(d Data) Data {
	raw, _ := json.Marshal(d)
	var out Data
	_ = json.Unmarshal(raw, &out)
	return out
}
func (s *Service) Snapshot() Data { s.mu.Lock(); defer s.mu.Unlock(); return clone(s.data) }
func (s *Service) save(d Data) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".workspace-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), s.path); err != nil {
		return err
	}
	s.data = d
	return nil
}
func validDate(v string) bool { _, e := time.Parse("2006-01-02", v); return e == nil }
func validateSettings(v Settings) error {
	a, e := time.Parse("15:04", v.Start)
	b, e2 := time.Parse("15:04", v.End)
	_, e3 := time.LoadLocation(v.Zone)
	if e != nil || e2 != nil || e3 != nil || !b.After(a) || v.Reserve < 20 || v.Reserve > 30 || v.Block < 30 || v.Block > 120 || v.Break < 10 || v.Break > 20 || v.SleepTarget < 4 || v.SleepTarget > 12 {
		return errors.New("Проверьте часы, часовой пояс, резерв 20–30%, блок 30–120 мин, перерыв 10–20 мин и цель сна 4–12 ч.")
	}
	return nil
}

var pipelineSubject = regexp.MustCompile(`^(.+?) \| Failed pipeline for .+ \| ([0-9a-fA-F]{7,40})$`)

func Candidates(state model.AppState, d Data) []Candidate {
	out := []Candidate{}
	seen := map[string]bool{}
	for _, key := range d.Dismissed {
		seen[key] = true
	}
	for _, t := range d.Tasks {
		seen[t.SourceKey] = true
	}
	add := func(source, identity, title, project string) {
		sum := sha256.Sum256([]byte(source + "\x00" + identity))
		key := fmt.Sprintf("%x", sum[:16])
		if !seen[key] {
			out = append(out, Candidate{Key: key, Title: title, Project: project, Source: source, Count: 1})
			seen[key] = true
		}
	}
	for _, v := range state.Reminders {
		add("reminders", v.List+"\x00"+v.Title, v.Title, v.List)
	}
	groups := map[string][]model.MailMessage{}
	order := []string{}
	for _, v := range state.Mail {
		identity := fmt.Sprintf("%d", v.ID)
		if v.ID == 0 {
			identity = v.Sender + v.Subject + v.Date.Format(time.RFC3339)
		}
		group := identity
		if m := pipelineSubject.FindStringSubmatch(v.Subject); m != nil {
			group = "pipeline:" + v.Sender + "\x00" + m[1] + "\x00" + strings.ToLower(m[2])
		}
		if _, ok := groups[group]; !ok {
			order = append(order, group)
		}
		groups[group] = append(groups[group], v)
	}
	for _, group := range order {
		messages := groups[group]
		// Respect earlier imports made before pipeline grouping was introduced.
		imported := false
		for _, v := range messages {
			identity := fmt.Sprintf("%d", v.ID)
			if v.ID == 0 {
				identity = v.Sender + v.Subject + v.Date.Format(time.RFC3339)
			}
			sum := sha256.Sum256([]byte("mail\x00" + identity))
			if seen[fmt.Sprintf("%x", sum[:16])] {
				imported = true
			}
		}
		if imported {
			continue
		}
		title := "Разобрать письмо: " + messages[0].Subject
		if m := pipelineSubject.FindStringSubmatch(messages[0].Subject); m != nil {
			title = "Проверить сбой сборки: " + m[1] + " · " + m[2]
		}
		before := len(out)
		add("mail", group, title, "")
		if len(out) > before {
			out[len(out)-1].Count = len(messages)
			details := []string{}
			for _, v := range messages {
				details = append(details, v.Subject+" · "+v.Sender+" · "+v.Date.Format("2006-01-02"))
			}
			out[len(out)-1].Details = strings.Join(details, "\n")
		}
	}
	for _, v := range state.Notes {
		add("notes", v.Folder+"\x00"+v.Title, "Разобрать заметку: "+v.Title, "")
	}
	for _, v := range state.Calendar {
		add("calendar", v.Calendar+v.Title+v.Start.Format(time.RFC3339), "Подготовиться: "+v.Title, "")
	}
	return out
}
func (s *Service) Apply(c Command, state model.AppState, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Version != s.data.Version {
		return ErrConflict
	}
	d := clone(s.data)
	switch c.Action {
	case "capture", "import":
		t := c.Task
		t.Source = "manual"
		t.SourceKey = ""
		t.Status = "inbox"
		t.MITDate = ""
		t.MainDate = ""
		t.Weekly = false
		if c.Action == "import" {
			found := false
			for _, v := range Candidates(state, d) {
				if v.Key == c.Key {
					t.Title = v.Title
					t.Project = v.Project
					t.Source = v.Source
					t.SourceKey = v.Key
					found = true
					break
				}
			}
			if !found {
				return errors.New("Материал уже разобран или больше не доступен.")
			}
		}
		t.ID = fmt.Sprintf("%d-%d", now.UnixNano(), d.Version)
		t.UpdatedAt = now
		if t.Minutes == 0 {
			t.Minutes = 90
		}
		d.Tasks = append(d.Tasks, t)
	case "update":
		found := false
		for i, t := range d.Tasks {
			if t.ID == c.Task.ID {
				v := c.Task
				v.ID = t.ID
				v.Source = t.Source
				v.SourceKey = t.SourceKey
				v.UpdatedAt = now
				if v.Status == "backlog" || v.Status == "waiting" || v.Status == "inbox" {
					v.MITDate = ""
				}
				if v.MainDate != v.MITDate {
					v.MainDate = ""
				}
				if v.Status == "waiting" && (strings.TrimSpace(v.WaitingOn) == "" || !validDate(v.CheckDate)) {
					return errors.New("Для ожидания укажите причину и дату следующей проверки.")
				}
				d.Tasks[i] = v
				found = true
				break
			}
		}
		if !found {
			return errors.New("Задача не найдена.")
		}
	case "main", "daily":
		if !validDate(c.Date) {
			return errors.New("Выберите дату плана.")
		}
		found := false
		for i := range d.Tasks {
			t := &d.Tasks[i]
			if t.ID == c.Task.ID {
				if t.Status != "ready" && t.Status != "doing" {
					return errors.New("Выберите подготовленное действие или задачу в работе.")
				}
				t.MITDate = c.Date
				t.MainDate = ""
				if c.Action == "main" {
					t.MainDate = c.Date
				}
				t.UpdatedAt = now
				found = true
			} else if c.Action == "main" && t.MainDate == c.Date {
				t.MainDate = ""
			}
		}
		if !found {
			return errors.New("Задача не найдена.")
		}
	case "dismiss":
		found := false
		for _, v := range Candidates(state, d) {
			if v.Key == c.Key {
				d.Dismissed = append(d.Dismissed, v.Key)
				found = true
				break
			}
		}
		if !found {
			return errors.New("Материал уже разобран или больше не доступен.")
		}
	case "restoreCandidates":
		d.Dismissed = nil
	case "settings":
		if err := validateSettings(c.Settings); err != nil {
			return err
		}
		d.Settings = c.Settings
	case "energy":
		if !validDate(c.Date) || (c.Energy != "auto" && c.Energy != "low" && c.Energy != "normal") {
			return errors.New("Некорректное состояние.")
		}
		d.Energy = c.Energy
		d.EnergyDate = c.Date
	case "review":
		if utf8.RuneCountInString(c.Results) > 4000 {
			return errors.New("Слишком длинные итоги обзора.")
		}
		d.Review = Review{now, c.Results}
	default:
		return errors.New("Неизвестное действие.")
	}
	doing, ready, weekly := 0, 0, 0
	previousWIP := 0
	for _, t := range s.data.Tasks {
		if t.Status == "doing" || t.Status == "waiting" {
			previousWIP++
		}
	}
	mits := map[string]int{}
	mains := map[string]int{}
	for _, t := range d.Tasks {
		if strings.TrimSpace(t.Title) == "" || utf8.RuneCountInString(t.Title) > 500 || utf8.RuneCountInString(t.Project) > 120 || t.Minutes < 15 || t.Minutes > 480 {
			return errors.New("Укажите действие (до 500 символов), проект и оценку 15–480 минут.")
		}
		if utf8.RuneCountInString(t.Resume) > 4000 || utf8.RuneCountInString(t.WaitingOn) > 500 || (t.CheckDate != "" && !validDate(t.CheckDate)) {
			return errors.New("Проверьте заметку, причину ожидания и дату проверки.")
		}
		if t.MainDate != "" {
			mains[t.MainDate]++
			if t.MainDate != t.MITDate || mains[t.MainDate] > 1 {
				return errors.New("На день можно выбрать один главный результат.")
			}
		}
		switch t.Status {
		case "inbox", "backlog", "done":
		case "ready":
			ready++
		case "doing", "waiting":
			doing++
		default:
			return errors.New("Неизвестный статус задачи.")
		}
		if t.MITDate != "" {
			if !validDate(t.MITDate) || (t.Status != "ready" && t.Status != "doing" && t.Status != "done") {
				return errors.New("В план можно выбрать подготовленные задачи или задачи в работе; завершённые результаты сохраняются в дне.")
			}
			mits[t.MITDate]++
			if mits[t.MITDate] > 3 {
				return errors.New("На день можно выбрать один главный и до двух дополнительных результатов. Сначала снимите с плана одну задачу.")
			}
		}
		if t.Weekly && t.Status != "done" {
			weekly++
		}
	}
	if doing > 3 && doing > previousWIP {
		return errors.New("В работе и ожидании уже 3 задачи. Завершите или явно отложите одну из них.")
	}
	if ready > 15 {
		return errors.New("Ready ≤ 15. Остальные действия оставьте в Backlog.")
	}
	if weekly > 5 {
		return errors.New("На неделю можно выбрать максимум 5 результатов.")
	}
	d.Version++
	return s.save(d)
}
func (s *Service) SetHealth(h Health, version int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if version != s.data.Version {
		return ErrConflict
	}
	d := clone(s.data)
	d.Health = &h
	d.Version++
	return s.save(d)
}

type Block struct {
	TaskID string    `json:"taskId"`
	Title  string    `json:"title"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Kind   string    `json:"kind"`
}
type Schedule struct {
	Date      string   `json:"date"`
	Blocks    []Block  `json:"blocks"`
	Warnings  []string `json:"warnings"`
	Available bool     `json:"available"`
	Free      int      `json:"free"`
	Budget    int      `json:"budget"`
	Planned   int      `json:"planned"`
	Mode      string   `json:"mode"`
	Reason    string   `json:"reason"`
}

func Build(d Data, state model.AppState, date string, now time.Time) Schedule {
	out := Schedule{Date: date, Blocks: []Block{}, Warnings: []string{}, Mode: "normal", Reason: "Обычная длительность блоков; резерв на непредвиденные задачи."}
	zone, _ := time.LoadLocation(d.Settings.Zone)
	if zone == nil || !validDate(date) {
		out.Warnings = append(out.Warnings, "Выберите корректную дату и часовой пояс.")
		return out
	}
	start, _ := time.ParseInLocation("2006-01-02 15:04", date+" "+d.Settings.Start, zone)
	end, _ := time.ParseInLocation("2006-01-02 15:04", date+" "+d.Settings.End, zone)
	today := now.In(zone).Format("2006-01-02")
	low := false
	if d.EnergyDate == date && d.Energy != "auto" {
		low = d.Energy == "low"
		out.Reason = "Нагрузка выбрана вами для этого дня."
	} else if date == today && d.Health != nil && d.Health.SleepHours != nil && now.Sub(d.Health.SleepEnd) < 36*time.Hour && !d.Health.SleepEnd.After(now) {
		low = *d.Health.SleepHours < d.Settings.SleepTarget
		out.Reason = "Сон сопоставлен с вашей целью. Это настройка нагрузки, а не оценка здоровья."
	} else {
		out.Reason = "Свежих данных сна нет. Нагрузку можно выбрать вручную."
	}
	block := d.Settings.Block
	if low {
		block = min(block, 45)
		out.Mode = "gentle"
	}
	if date < today {
		out.Warnings = append(out.Warnings, "Выбрана прошедшая дата.")
		return out
	}
	if date == today && now.After(start) {
		start = now.Truncate(time.Minute).Add(time.Minute)
	}
	if !start.Before(end) {
		out.Warnings = append(out.Warnings, "Рабочий день завершён. Выберите следующий день.")
		return out
	}
	calendarOK := false
	for _, src := range state.Sources {
		if src.Name == "calendar" {
			calendarOK = src.OK && !src.UpdatedAt.IsZero() && now.Sub(src.UpdatedAt) < 24*time.Hour && !src.UpdatedAt.After(now.Add(time.Minute))
		}
	}
	if !calendarOK {
		out.Warnings = append(out.Warnings, "Календарь недоступен или старше 24 часов. Обновите его перед выделением временных блоков.")
		return out
	}
	// Calendar collector covers a bounded window; never assume other days are empty.
	if state.CalendarFrom.IsZero() || start.Before(state.CalendarFrom) || end.After(state.CalendarTo) {
		out.Warnings = append(out.Warnings, "Нет подтверждённого диапазона календаря на выбранную дату. Обновите источники; MIT сохранены.")
		return out
	}
	out.Available = true
	busy := []Block{}
	for _, e := range state.Calendar {
		if e.End.After(start) && e.Start.Before(end) {
			a, b := e.Start, e.End
			if a.Before(start) {
				a = start
			}
			if b.After(end) {
				b = end
			}
			if b.After(a) {
				busy = append(busy, Block{Title: e.Title, Start: a, End: b, Kind: "calendar"})
			}
		}
	}
	sort.Slice(busy, func(i, j int) bool { return busy[i].Start.Before(busy[j].Start) })
	free := []Block{}
	cursor := start
	for _, b := range busy {
		if b.Start.After(cursor) {
			free = append(free, Block{Start: cursor, End: b.Start})
		}
		if b.End.After(cursor) {
			cursor = b.End
		}
		out.Blocks = append(out.Blocks, b)
	}
	if cursor.Before(end) {
		free = append(free, Block{Start: cursor, End: end})
	}
	for _, f := range free {
		out.Free += int(f.End.Sub(f.Start).Minutes())
	}
	out.Budget = out.Free * (100 - d.Settings.Reserve) / 100
	remaining := out.Budget
	slot := 0
	var pos time.Time
	if len(free) > 0 {
		pos = free[0].Start
	}
	ordered := append([]Task(nil), d.Tasks...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].MainDate == date && ordered[j].MainDate != date })
	for _, t := range ordered {
		if t.MITDate != date || (t.Status != "ready" && t.Status != "doing") {
			continue
		}
		left := t.Minutes
		for left > 0 && slot < len(free) && remaining >= 15 {
			duration := min(left, block)
			span := int(free[slot].End.Sub(pos).Minutes())
			duration = min(duration, span, remaining)
			if duration < 15 {
				slot++
				if slot < len(free) {
					pos = free[slot].Start
				}
				continue
			}
			stop := pos.Add(time.Duration(duration) * time.Minute)
			out.Blocks = append(out.Blocks, Block{t.ID, t.Title, pos, stop, "focus"})
			out.Planned += duration
			remaining -= duration
			left -= duration
			pos = stop
			pause := min(d.Settings.Break, int(free[slot].End.Sub(pos).Minutes()), remaining)
			if pause > 0 {
				stop = pos.Add(time.Duration(pause) * time.Minute)
				out.Blocks = append(out.Blocks, Block{Title: "Перерыв", Start: pos, End: stop, Kind: "break"})
				pos = stop
				remaining -= pause
			}
		}
		if left > 0 {
			out.Warnings = append(out.Warnings, fmt.Sprintf("Не помещается: %s — %d мин. Уменьшите объём результата или перенесите задачу.", t.Title, left))
		}
	}
	sort.SliceStable(out.Blocks, func(i, j int) bool { return out.Blocks[i].Start.Before(out.Blocks[j].Start) })
	return out
}
