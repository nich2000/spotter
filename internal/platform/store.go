// Package platform contains PostgreSQL, HTTP and broker adapters.
package platform

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"spotter/internal/core"
)

//go:embed schema.sql
var schema string

type Database interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Ping(context.Context) error
	Close()
}
type Store struct{ DB Database }

func Open(ctx context.Context, url string) (*Store, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(871203)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, schema); err != nil {
		return err
	}
	b, _ := json.Marshal(core.NewState())
	if _, err = tx.Exec(ctx, "INSERT INTO workspace VALUES(1,$1) ON CONFLICT DO NOTHING", b); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) Read(ctx context.Context) (core.State, error) {
	var state core.State
	var b []byte
	err := s.DB.QueryRow(ctx, "SELECT state FROM workspace WHERE id=1").Scan(&b)
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(b, &state)
	if err == nil && state.SchemaVersion != 2 {
		err = core.Fail("SCHEMA_UNSUPPORTED", 503, "Неподдерживаемая схема")
	}
	return state, err
}
func hash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func (s *Store) Execute(ctx context.Context, c core.Command) (core.Receipt, error) {
	var receipt core.Receipt
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return receipt, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT state FROM workspace WHERE id=1 FOR UPDATE").Scan(&raw); err != nil {
		return receipt, err
	}
	canonical, _ := json.Marshal(c)
	digest := hash(canonical)
	var priorHash string
	var prior []byte
	err = tx.QueryRow(ctx, "SELECT hash,receipt FROM operations WHERE id=$1", c.OperationID).Scan(&priorHash, &prior)
	if err == nil {
		if priorHash != digest {
			return receipt, core.Fail("IDEMPOTENCY_KEY_REUSED", 409, "Этот operationId использован с другим запросом")
		}
		err = json.Unmarshal(prior, &receipt)
		return receipt, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return receipt, err
	}
	var state core.State
	if err = json.Unmarshal(raw, &state); err != nil {
		return receipt, err
	}
	if c.Command == "wellbeing.save" {
		var allowed bool
		if err = tx.QueryRow(ctx, "SELECT enabled FROM consents WHERE category='diary' FOR UPDATE").Scan(&allowed); err != nil {
			return receipt, err
		}
		if !allowed {
			return receipt, core.Fail("CONSENT_REQUIRED", 403, "Согласие на дневник отозвано")
		}
	}
	next, receipt, err := core.Apply(state, c, time.Now())
	if err != nil {
		return receipt, err
	}
	raw, err = json.Marshal(next)
	if err != nil {
		return receipt, err
	}
	result, _ := json.Marshal(receipt)
	if _, err = tx.Exec(ctx, "UPDATE workspace SET state=$1 WHERE id=1", raw); err != nil {
		return receipt, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO operations VALUES($1,$2,$3)", c.OperationID, digest, result); err != nil {
		return receipt, err
	}
	if next.Revision != state.Revision {
		if _, err = tx.Exec(ctx, "INSERT INTO outbox(revision) VALUES($1)", next.Revision); err != nil {
			return receipt, err
		}
	}
	return receipt, tx.Commit(ctx)
}
func (s *Store) Snapshot(ctx context.Context, token string) (core.State, string, error) {
	if token != "" {
		var b []byte
		err := s.DB.QueryRow(ctx, "SELECT state FROM read_snapshots WHERE token=$1 AND expires_at>now()", token).Scan(&b)
		if errors.Is(err, pgx.ErrNoRows) {
			return core.State{}, "", core.Fail("READ_TOKEN_EXPIRED", 410, "Снимок истёк; перечитайте данные")
		}
		if err != nil {
			return core.State{}, "", err
		}
		var state core.State
		err = json.Unmarshal(b, &state)
		return state, token, err
	}
	state, err := s.Read(ctx)
	if err != nil {
		return state, "", err
	}
	b, _ := json.Marshal(state)
	token = core.ID()
	_, err = s.DB.Exec(ctx, "INSERT INTO read_snapshots(token,state) VALUES($1,$2)", token, b)
	return state, token, err
}

// ImportLegacy only imports into an unused workspace and records its source checksum.
func (s *Store) ImportLegacy(ctx context.Context, raw []byte, dryRun bool) (core.State, error) {
	next, err := core.MigrateLegacy(raw)
	if err != nil {
		return next, err
	}
	if dryRun {
		return next, nil
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return next, err
	}
	defer tx.Rollback(ctx)
	var b []byte
	if err = tx.QueryRow(ctx, "SELECT state FROM workspace WHERE id=1 FOR UPDATE").Scan(&b); err != nil {
		return next, err
	}
	var old core.State
	if err = json.Unmarshal(b, &old); err != nil {
		return next, err
	}
	if old.Migration["sourceSHA256"] == next.Migration["sourceSHA256"] {
		return old, nil
	}
	if old.Revision != 0 || len(old.Tasks) > 0 || len(old.Projects) > 0 || len(old.Days) > 0 {
		return next, core.Fail("WORKSPACE_NOT_EMPTY", 409, "Миграция разрешена только в пустое workspace")
	}
	b, err = json.Marshal(next)
	if err != nil {
		return next, err
	}
	if _, err = tx.Exec(ctx, "UPDATE workspace SET state=$1 WHERE id=1", b); err != nil {
		return next, err
	}
	return next, tx.Commit(ctx)
}
