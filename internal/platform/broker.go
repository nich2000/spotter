package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"
	"spotter/internal/core"
	"spotter/internal/model"
	"spotter/internal/workspace"
)

type Batch struct {
	Health            *workspace.Health `json:"health,omitempty"`
	ConsentGeneration int64             `json:"consentGeneration,omitempty"`
	SchemaVersion     int               `json:"schemaVersion"`
	BatchID           string            `json:"batchId"`
	Source            string            `json:"sourceId"`
	Sequence          int64             `json:"sequence"`
	ObservedAt        time.Time         `json:"observedAt"`
	OK                bool              `json:"ok"`
	Error             string            `json:"error,omitempty"`
	Data              model.SourceData  `json:"data"`
}
type queuedBatch struct {
	DeviceID string `json:"deviceId"`
	Batch    Batch  `json:"batch"`
}
type Broker struct {
	Conn *nats.Conn
	JS   nats.JetStreamContext
}

func ConnectBroker(rawURL string) (*Broker, error) {
	nc, err := nats.Connect(rawURL, nats.Timeout(5*time.Second), nats.MaxReconnects(-1))
	if err != nil {
		return nil, err
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, err
	}
	return &Broker{Conn: nc, JS: js}, nil
}
func (b *Broker) Init() error {
	_, err := b.JS.AddStream(&nats.StreamConfig{Name: "SPOTTER_IMPORT", Subjects: []string{"spotter.import"}, Storage: nats.FileStorage, Retention: nats.WorkQueuePolicy, MaxBytes: 128 << 20, MaxMsgSize: 256 << 10, Discard: nats.DiscardNew})
	return err
}
func ValidateBatch(b Batch) error {
	if b.SchemaVersion != 1 || b.BatchID == "" || len(b.BatchID) > 128 || b.Sequence < 1 || b.ObservedAt.IsZero() || b.ObservedAt.After(time.Now().Add(5*time.Minute)) {
		return core.Fail("VALIDATION_FAILED", 422, "Некорректный пакет")
	}
	switch b.Source {
	case "calendar", "reminders", "mail", "notes", "health":
	default:
		return core.Fail("VALIDATION_FAILED", 422, "Неизвестный источник")
	}
	if b.Source == "health" {
		if b.Health == nil || b.ConsentGeneration < 1 || b.Health.MeasuredAt.IsZero() || b.Health.MeasuredAt.After(time.Now().Add(5*time.Minute)) || b.Health.SleepHours != nil && (*b.Health.SleepHours < 0 || *b.Health.SleepHours > 24) {
			return core.Fail("VALIDATION_FAILED", 422, "Некорректный агрегат сна")
		}
	} else if b.Health != nil {
		return core.Fail("VALIDATION_FAILED", 422, "Здоровье разрешено только в своей категории")
	}
	// Collectors emit an authoritative snapshot, which may contain many small records.
	// The ingestion boundary separately enforces the 256 KiB serialized message limit.
	if len(b.Data.Calendar)+len(b.Data.Reminders)+len(b.Data.Mail)+len(b.Data.Notes) > 10000 {
		return core.Fail("VALIDATION_FAILED", 422, "Не более 10000 записей в пакете")
	}
	if b.Source != "calendar" && len(b.Data.Calendar) > 0 || b.Source != "reminders" && len(b.Data.Reminders) > 0 || b.Source != "mail" && len(b.Data.Mail) > 0 || b.Source != "notes" && len(b.Data.Notes) > 0 {
		return core.Fail("VALIDATION_FAILED", 422, "Пакет содержит чужой источник")
	}
	if b.Source == "calendar" && b.OK {
		if b.Data.CalendarFrom.IsZero() || !b.Data.CalendarTo.After(b.Data.CalendarFrom) {
			return core.Fail("VALIDATION_FAILED", 422, "Не указано окно календаря")
		}
		for _, event := range b.Data.Calendar {
			if event.Start.IsZero() || !event.End.After(event.Start) || event.Availability != "" && event.Availability != "free" && event.Availability != "busy" {
				return core.Fail("VALIDATION_FAILED", 422, "Некорректное событие календаря")
			}
		}
	}
	return nil
}
func (a *API) ingestion(w http.ResponseWriter, r *http.Request) {
	device := r.Context().Value(deviceKey{}).(string)
	if r.Method == "GET" && r.URL.Path == "/api/v2/integrations/consents" {
		var enabled bool
		var generation int64
		err := a.Store.DB.QueryRow(r.Context(), "SELECT enabled,generation FROM consents WHERE category='health'").Scan(&enabled, &generation)
		if err != nil {
			problem(w, err)
			return
		}
		send(w, 200, core.Object{"enabled": enabled, "generation": generation})
		return
	}

	if r.Method == "GET" {
		id := strings.TrimPrefix(r.URL.Path, "/api/v2/integrations/batches/")
		var state string
		var message *string
		var applied *time.Time
		err := a.Store.DB.QueryRow(r.Context(), "SELECT state,error,applied_at FROM batches WHERE id=$1 AND device_id=$2", id, device).Scan(&state, &message, &applied)
		if err != nil {
			problem(w, core.Fail("ENTITY_NOT_FOUND", 404, "Квитанция не найдена"))
			return
		}
		send(w, 200, core.Object{"batchId": id, "state": state, "error": message, "appliedAt": applied})
		return
	}
	if r.Method != "POST" || r.URL.Path != "/api/v2/integrations/batches" {
		w.WriteHeader(405)
		return
	}
	var b Batch
	if err := body(w, r, &b); err != nil {
		problem(w, err)
		return
	}
	if err := ValidateBatch(b); err != nil {
		problem(w, err)
		return
	}
	if r.Header.Get("Idempotency-Key") != b.BatchID {
		problem(w, core.Fail("VALIDATION_FAILED", 422, "Idempotency-Key должен совпадать с batchId"))
		return
	}
	if b.Source == "health" {
		var allowed bool
		err := a.Store.DB.QueryRow(r.Context(), "SELECT enabled AND generation=$1 FROM consents WHERE category='health'", b.ConsentGeneration).Scan(&allowed)
		if err != nil || !allowed {
			problem(w, core.Fail("CONSENT_REQUIRED", 403, "Согласие на здоровье отсутствует или изменилось"))
			return
		}
	}
	raw, _ := json.Marshal(b)
	digest := hash(raw)
	queued, _ := json.Marshal(queuedBatch{DeviceID: device, Batch: b})
	if len(queued) > 256<<10 {
		problem(w, core.Fail("REQUEST_TOO_LARGE", 413, "Пакет больше 256 KiB"))
		return
	}
	_, err := a.Store.DB.Exec(r.Context(), "INSERT INTO batches(id,device_id,hash,state) VALUES($1,$2,$3,'pending') ON CONFLICT DO NOTHING", b.BatchID, device, digest)
	if err != nil {
		problem(w, err)
		return
	}
	var storedHash, owner, state string
	err = a.Store.DB.QueryRow(r.Context(), "SELECT hash,device_id,state FROM batches WHERE id=$1", b.BatchID).Scan(&storedHash, &owner, &state)
	if err != nil {
		problem(w, err)
		return
	}
	if owner != device || storedHash != digest {
		problem(w, core.Fail("IDEMPOTENCY_KEY_REUSED", 409, "Пакет уже зарегистрирован с другим содержимым"))
		return
	}
	if state == "applied" || state == "rejected" {
		send(w, 200, core.Object{"batchId": b.BatchID, "state": state})
		return
	}
	if a.Broker == nil {
		problem(w, core.Fail("SERVICE_UNAVAILABLE", 503, "Брокер недоступен"))
		return
	}
	_, err = a.Broker.JS.Publish("spotter.import", queued, nats.MsgId(b.BatchID), nats.Context(r.Context()))
	if err != nil {
		problem(w, err)
		return
	}
	_, err = a.Store.DB.Exec(r.Context(), "UPDATE batches SET state='received' WHERE id=$1 AND state='pending'", b.BatchID)
	if err != nil {
		problem(w, err)
		return
	}
	send(w, 202, core.Object{"batchId": b.BatchID, "state": "received"})
}
func (s *Store) ApplyBatch(ctx context.Context, q queuedBatch) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var active bool
	if err = tx.QueryRow(ctx, "SELECT active FROM devices WHERE id=$1 FOR UPDATE", q.DeviceID).Scan(&active); err != nil {
		return err
	}
	var state, digest string
	if err = tx.QueryRow(ctx, "SELECT state,hash FROM batches WHERE id=$1 AND device_id=$2 FOR UPDATE", q.Batch.BatchID, q.DeviceID).Scan(&state, &digest); err != nil {
		return err
	}
	if state == "applied" || state == "rejected" {
		return nil
	}
	raw, _ := json.Marshal(q.Batch)
	if hash(raw) != digest {
		return fmt.Errorf("batch hash mismatch")
	}
	if q.Batch.Source == "health" {
		var allowed bool
		if err = tx.QueryRow(ctx, "SELECT enabled AND generation=$1 FROM consents WHERE category='health' FOR UPDATE", q.Batch.ConsentGeneration).Scan(&allowed); err != nil {
			return err
		}
		active = active && allowed
	}
	if !active {
		_, err = tx.Exec(ctx, "UPDATE batches SET state='rejected',error='DEVICE_REVOKED' WHERE id=$1", q.Batch.BatchID)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	var seq int64
	err = tx.QueryRow(ctx, "SELECT sequence FROM sources WHERE device_id=$1 AND name=$2", q.DeviceID, q.Batch.Source).Scan(&seq)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if q.Batch.Sequence > seq && (q.Batch.OK || q.Batch.Source == "calendar") && q.Batch.Source != "health" {
		var workspaceRaw []byte
		if err = tx.QueryRow(ctx, "SELECT state FROM workspace WHERE id=1 FOR UPDATE").Scan(&workspaceRaw); err != nil {
			return err
		}
		var work core.State
		if err = json.Unmarshal(workspaceRaw, &work); err != nil {
			return err
		}
		if work.Candidates == nil {
			work.Candidates = map[string]core.Object{}
		}
		source := model.AppState{Calendar: q.Batch.Data.Calendar, Reminders: q.Batch.Data.Reminders, Mail: q.Batch.Data.Mail, Notes: q.Batch.Data.Notes}
		for _, candidate := range workspace.Candidates(source, workspace.Data{}) {
			id := hash([]byte(q.DeviceID + "/" + candidate.Key))
			old := work.Candidates[id]
			if old == nil {
				old = core.Object{"id": id, "state": "pending", "candidateVersion": float64(1), "acceptedOccurrenceId": nil}
				work.Candidates[id] = old
			}
			old["proposedTitle"] = candidate.Title
			old["sourceId"] = q.Batch.Source
			old["sourceSnapshotId"] = q.Batch.BatchID
			old["identityMethod"] = "legacy_fingerprint"
			old["details"] = candidate.Details
		}

		if q.Batch.Source == "calendar" {
			if work.CalendarSources == nil {
				work.CalendarSources = map[string]core.Object{}
			}
			key := q.DeviceID + "/calendar"
			old := work.CalendarSources[key]
			if old == nil {
				old = core.Object{"id": key, "events": []any{}}
			}
			old["ok"] = q.Batch.OK
			old["error"] = q.Batch.Error
			if q.Batch.OK {
				rows := []any{}
				for _, e := range q.Batch.Data.Calendar {
					identity := e.ID
					if identity == "" {
						identity = e.Calendar + "/" + e.Title
					}
					id := hash([]byte(key + "/" + identity + "/" + e.Start.Format(time.RFC3339)))
					rows = append(rows, core.Object{"id": id, "title": e.Title, "startAt": e.Start.Format(time.RFC3339), "endAt": e.End.Format(time.RFC3339), "source": e.Calendar, "kind": "external", "allDay": e.AllDay, "availability": e.Availability, "cancelled": e.Cancelled})
				}
				// Replace only the authoritative covered window, retain history outside it.
				for _, raw := range core.Copy(old)["events"].([]any) {
					e, ok := raw.(map[string]any)
					if !ok {
						continue
					}
					start, _ := time.Parse(time.RFC3339, stringValue(e, "startAt"))
					end, _ := time.Parse(time.RFC3339, stringValue(e, "endAt"))
					if !start.Before(q.Batch.Data.CalendarTo) || !end.After(q.Batch.Data.CalendarFrom) {
						rows = append(rows, e)
					}
				}
				old["events"] = rows
				old["observedAt"] = q.Batch.ObservedAt.Format(time.RFC3339)
				old["from"] = q.Batch.Data.CalendarFrom.Format(time.RFC3339)
				old["to"] = q.Batch.Data.CalendarTo.Format(time.RFC3339)
			}
			work.CalendarSources[key] = old
			work.Revision++
		}
		work.SourceSnapshotID = q.Batch.BatchID
		workspaceRaw, err = json.Marshal(work)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE workspace SET state=$1 WHERE id=1", workspaceRaw); err != nil {
			return err
		}
	}
	if q.Batch.Sequence > seq {
		_, err = tx.Exec(ctx, "INSERT INTO sources(device_id,name,sequence,snapshot) VALUES($1,$2,$3,$4) ON CONFLICT(device_id,name) DO UPDATE SET sequence=excluded.sequence,snapshot=excluded.snapshot,updated_at=now()", q.DeviceID, q.Batch.Source, q.Batch.Sequence, raw)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, "UPDATE batches SET state='applied',applied_at=now() WHERE id=$1", q.Batch.BatchID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "UPDATE devices SET last_seen=now() WHERE id=$1", q.DeviceID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (b *Broker) Run(ctx context.Context, s *Store) error {
	sub, err := b.JS.PullSubscribe("spotter.import", "spotter-worker", nats.BindStream("SPOTTER_IMPORT"), nats.ManualAck(), nats.AckExplicit(), nats.MaxDeliver(10))
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		msgs, err := sub.Fetch(1, nats.MaxWait(time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				continue
			}
			return err
		}
		for _, m := range msgs {
			var q queuedBatch
			if err = core.Decode(m.Data, &q); err != nil {
				_ = m.Term()
				continue
			}
			if err = s.ApplyBatch(ctx, q); err != nil {
				_ = m.NakWithDelay(5 * time.Second)
				continue
			}
			_ = m.AckSync()
		}
		_, _ = s.DB.Exec(ctx, "DELETE FROM read_snapshots WHERE expires_at<now()")
	}
}
func (a *API) sources(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Store.DB.Query(r.Context(), "SELECT device_id,name,snapshot,updated_at FROM sources ORDER BY name")
	if err != nil {
		problem(w, err)
		return
	}
	defer rows.Close()
	items := []core.Object{}
	for rows.Next() {
		var device, name string
		var b []byte
		var updated time.Time
		if err = rows.Scan(&device, &name, &b, &updated); err != nil {
			problem(w, err)
			return
		}
		items = append(items, core.Object{"deviceId": device, "name": name, "snapshot": json.RawMessage(b), "receivedAt": updated})
	}
	send(w, 200, core.Object{"items": items})
}
