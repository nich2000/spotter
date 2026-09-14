package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Only sleep intervals are accepted; raw samples are never persisted or sent to AI.
type SleepSample struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
type HealthInput struct {
	MeasuredAt time.Time     `json:"measuredAt"`
	Sleep      []SleepSample `json:"sleep"`
}
type Health struct {
	MeasuredAt time.Time `json:"measuredAt"`
	ReceivedAt time.Time `json:"receivedAt"`
	SleepEnd   time.Time `json:"sleepEnd"`
	SleepHours *float64  `json:"sleepHours"`
	Source     string    `json:"source"`
}

func ParseHealth(r io.Reader, now time.Time) (Health, error) {
	var in HealthInput
	dec := json.NewDecoder(io.LimitReader(r, 2<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return Health{}, errors.New("Не удалось прочитать health.json. Проверьте формат команды на iPhone.")
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return Health{}, errors.New("Лишние данные в health.json.")
	}
	if in.MeasuredAt.IsZero() || in.MeasuredAt.After(now.Add(5*time.Minute)) || len(in.Sleep) > 5000 {
		return Health{}, errors.New("Некорректная дата измерения или слишком много интервалов сна.")
	}
	out := Health{MeasuredAt: in.MeasuredAt, ReceivedAt: now, Source: "Apple Health · iPhone / iCloud Drive"}
	cutoff := in.MeasuredAt.Add(-24 * time.Hour)
	spans := []SleepSample{}
	for _, v := range in.Sleep {
		if v.Start.IsZero() || !v.End.After(v.Start) || v.End.After(in.MeasuredAt.Add(time.Minute)) || v.End.Sub(v.Start) > 24*time.Hour {
			return Health{}, errors.New("Некорректный интервал сна.")
		}
		if !v.End.After(cutoff) {
			continue
		}
		if v.Start.Before(cutoff) {
			v.Start = cutoff
		}
		spans = append(spans, v)
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start.Before(spans[j].Start) })
	var duration time.Duration
	var start, end time.Time
	for _, v := range spans {
		if start.IsZero() {
			start, end = v.Start, v.End
			continue
		}
		if v.Start.After(end) {
			duration += end.Sub(start)
			start, end = v.Start, v.End
		} else if v.End.After(end) {
			end = v.End
		}
	}
	if !start.IsZero() {
		duration += end.Sub(start)
		hours := duration.Hours()
		out.SleepHours = &hours
		out.SleepEnd = end
	}
	return out, nil
}

type Bridge struct {
	Path    string
	Service *Service
	mu      sync.Mutex
	last    [32]byte
	Status  string
}

func DefaultHealthPath() string {
	if v := os.Getenv("SPOTTER_HEALTH_FILE"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library/Mobile Documents/com~apple~CloudDocs/Spotter/health.json")
}
func (b *Bridge) Message() string { b.mu.Lock(); defer b.mu.Unlock(); return b.Status }
func (b *Bridge) Sync(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	f, err := os.Open(b.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			b.Status = "Ожидаем файл от iPhone. Настройте команду по инструкции."
		} else {
			b.Status = "Не удалось прочитать файл iCloud Drive."
		}
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		b.Status = "Файл здоровья недоступен или больше 2 МБ."
		return
	}
	sum := sha256.Sum256(raw)
	if sum == b.last {
		b.Status = "Файл синхронизирован. Ожидаем следующий экспорт iPhone."
		return
	}
	h, err := ParseHealth(bytes.NewReader(raw), now)
	if err != nil {
		b.Status = err.Error()
		return
	}
	current := b.Service.Snapshot()
	if current.Health != nil && !h.MeasuredAt.After(current.Health.MeasuredAt) {
		b.last = sum
		b.Status = "Файл не новее сохранённых данных."
		return
	}
	if err = b.Service.SetHealth(h, current.Version); err != nil {
		b.Status = "Импорт отложен; повторим через минуту."
		return
	}
	b.last = sum
	b.Status = "Данные iPhone получены автоматически."
}
func (b *Bridge) Run(ctx context.Context) {
	b.Sync(time.Now())
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			b.Sync(now)
		}
	}
}
