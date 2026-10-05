//go:build integration

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"spotter/internal/core"
	"spotter/internal/model"
)

func TestIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dsn := os.Getenv("SPOTTER_TEST_DATABASE")
	if dsn == "" {
		t.Fatal("SPOTTER_TEST_DATABASE required")
	}
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schemaName := "test_" + core.ID()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP SCHEMA "+schemaName+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schemaName
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := &Store{DB: pool}
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(ctx); err != nil {
		t.Fatal("repeat migration", err)
	}
	broker, err := ConnectBroker(os.Getenv("SPOTTER_TEST_NATS"))
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Conn.Close()
	if err = broker.Init(); err != nil {
		t.Fatal(err)
	}
	files, err := OpenFiles(os.Getenv("S3_ENDPOINT"), os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	files.Bucket = "test-" + core.ID()
	if err = files.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer files.Client.RemoveBucket(context.Background(), files.Bucket)
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() { workerDone <- broker.Run(workerCtx, store) }()
	defer func() { stopWorker(); <-workerDone }()
	api := &API{Store: store, Broker: broker, Files: files, Assets: t.TempDir()}
	srv := httptest.NewUnstartedServer(nil)
	api.Origin = "http://" + srv.Listener.Addr().String()
	srv.Config.Handler = api.Handler()
	srv.Start()
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	request := func(method, path string, value any, headers map[string]string) (int, map[string]any) {
		t.Helper()
		var body io.Reader
		if value != nil {
			body = bytes.NewReader(raw(value))
		}
		req, _ := http.NewRequest(method, srv.URL+path, body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Spotter-Request", "workspace")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var data map[string]any
		if err = json.NewDecoder(res.Body).Decode(&data); err != nil {
			t.Fatal(path, err)
		}
		return res.StatusCode, data
	}
	status, _ := request("GET", "/api/v2/workspace", nil, nil)
	if status != 401 {
		t.Fatal("anonymous", status)
	}
	status, _ = request("POST", "/api/v2/auth/setup", map[string]string{"password": "isolated-test-password"}, nil)
	if status != 200 {
		t.Fatal("setup", status)
	}
	status, _ = request("POST", "/api/v2/workspace/commands", core.Command{OperationID: "forbidden", Command: "task.create", Payload: core.Object{"title": "blocked"}}, map[string]string{"Origin": "https://foreign.invalid"})
	if status != 403 {
		t.Fatal("origin", status)
	}
	command := core.Command{OperationID: "create", Command: "task.create", Payload: core.Object{"title": "Integration fixture"}}
	status, data := request("POST", "/api/v2/workspace/commands", command, nil)
	if status != 200 {
		t.Fatal(data)
	}
	id := data["result"].(map[string]any)["taskId"].(string)
	_, again := request("POST", "/api/v2/workspace/commands", command, nil)
	if again["result"].(map[string]any)["taskId"] != id {
		t.Fatal("replay changed id")
	}
	command.Payload["title"] = "changed"
	status, _ = request("POST", "/api/v2/workspace/commands", command, nil)
	if status != 409 {
		t.Fatal("reuse", status)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := store.Execute(ctx, core.Command{OperationID: fmt.Sprintf("race-%d", i), ExpectedRevision: 1, Command: "task.save", Payload: core.Object{"taskId": id, "patch": core.Object{"resume": fmt.Sprint(i)}}})
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("optimistic lock", success)
	}
	before, _ := store.Read(ctx)
	_, err = store.Execute(ctx, core.Command{OperationID: "rollback", ExpectedRevision: before.Revision, Command: "task.save", Payload: core.Object{"taskId": id, "patch": core.Object{"title": "must rollback"}, "planChanges": []any{core.Object{"planId": "missing", "action": "upsert"}}}})
	if err == nil {
		t.Fatal("invalid plan accepted")
	}
	after, _ := store.Read(ctx)
	if before.Tasks[id]["title"] != after.Tasks[id]["title"] || after.Revision != before.Revision {
		t.Fatal("partial commit")
	}
	snapshot, readToken, err := store.Snapshot(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Execute(ctx, core.Command{OperationID: "later", ExpectedRevision: after.Revision, Command: "task.create", Payload: core.Object{"title": "Later"}})
	if err != nil {
		t.Fatal(err)
	}
	frozen, _, err := store.Snapshot(ctx, readToken)
	if err != nil || len(frozen.Tasks) != len(snapshot.Tasks) {
		t.Fatal("snapshot drift")
	}
	// Planning persists through HTTP/PostgreSQL and replays its receipt without a second event.
	currentPlanning, _ := store.Read(ctx)
	otherReceipt, e := store.Execute(ctx, core.Command{OperationID: core.ID(), ExpectedRevision: currentPlanning.Revision, Command: "task.create", Payload: core.Object{"title": "Linked planning fixture"}})
	if e != nil {
		t.Fatal(e)
	}
	otherID := otherReceipt.Result["taskId"].(string)
	planningCommand := core.Command{OperationID: core.ID(), ExpectedRevision: otherReceipt.CommittedRevision, Command: "task.save", Payload: core.Object{"taskId": id, "patch": core.Object{
		"scheduled":      core.Object{"kind": "date_range", "startDate": "2026-10-05", "endDate": "2026-10-07", "zone": "Europe/Moscow"},
		"deadline":       core.Object{"kind": "date", "date": "2026-10-06", "zone": "Europe/Moscow"},
		"relatedTaskIds": []any{otherID},
	}}}
	status, planningReceipt := request("POST", "/api/v2/workspace/commands", planningCommand, nil)
	if status != 200 || len(planningReceipt["warnings"].([]any)) != 1 {
		t.Fatal("planning save", status, planningReceipt)
	}
	persisted, _ := store.Read(ctx)
	if persisted.Tasks[otherID]["relatedTaskIds"].([]any)[0] != id {
		t.Fatal("reverse link not persisted")
	}
	status, replay := request("POST", "/api/v2/workspace/commands", planningCommand, nil)
	replayed, _ := store.Read(ctx)
	if status != 200 || replay["committedRevision"] != planningReceipt["committedRevision"] || len(replayed.Events) != len(persisted.Events) {
		t.Fatal("planning replay", replay)
	}
	stale := planningCommand
	stale.OperationID = core.ID()
	status, _ = request("POST", "/api/v2/workspace/commands", stale, nil)
	if status != 409 {
		t.Fatal("stale planning accepted", status)
	}
	clear := core.Command{OperationID: core.ID(), ExpectedRevision: persisted.Revision, Command: "task.save", Payload: core.Object{"taskId": id, "patch": core.Object{"scheduled": nil}}}
	status, _ = request("POST", "/api/v2/workspace/commands", clear, nil)
	cleared, _ := store.Read(ctx)
	if status != 200 || cleared.Tasks[id]["scheduled"] != nil || cleared.Tasks[id]["deadline"] == nil {
		t.Fatal("clear changed deadline")
	}
	_, enrollment := request("POST", "/api/v2/device-enrollments", core.Object{}, nil)
	status, device := request("POST", "/api/v2/devices/enroll", core.Object{"code": enrollment["code"], "name": "Fixture Mac"}, nil)
	if status != 200 {
		t.Fatal(device)
	}
	// JetStream retains deduplication IDs across consecutive test runs.
	batchID := "integration-" + core.ID()
	headers := map[string]string{"Authorization": "Bearer " + device["token"].(string), "Idempotency-Key": batchID}
	batch := Batch{SchemaVersion: 1, BatchID: batchID, Source: "notes", Sequence: 1, ObservedAt: time.Now(), OK: true, Data: model.SourceData{Notes: []model.Note{{Title: "Candidate", Folder: "Test"}}}}
	status, data = request("POST", "/api/v2/integrations/batches", batch, headers)
	if status != 202 {
		t.Fatal("ingress", status, data)
	}
	for deadline := time.Now().Add(10 * time.Second); ; {
		_, receipt := request("GET", "/api/v2/integrations/batches/"+batch.BatchID, nil, headers)
		if receipt["state"] == "applied" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker timeout", receipt)
		}
		time.Sleep(50 * time.Millisecond)
	}
	status, _ = request("POST", "/api/v2/integrations/batches", batch, headers)
	if status != 200 {
		t.Fatal("batch replay", status)
	}
	current, _ := store.Read(ctx)
	if len(current.Candidates) != 1 {
		t.Fatal("candidate dedup", len(current.Candidates))
	}
	var candidate core.Object
	for _, v := range current.Candidates {
		candidate = v
	}
	accept := core.Command{OperationID: "accept", ExpectedRevision: current.Revision, Command: "source.accept", Payload: core.Object{"candidateId": candidate["id"], "candidateVersion": candidate["candidateVersion"], "sourceSnapshotId": candidate["sourceSnapshotId"]}}
	if _, err = store.Execute(ctx, accept); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Execute(ctx, accept); err != nil {
		t.Fatal("accept replay", err)
	}

	// Calendar blocks are atomic, idempotent and separate from task/plan facts.
	calState, _ := store.Read(ctx)
	calTask, err := store.Execute(ctx, core.Command{OperationID: core.ID(), ExpectedRevision: calState.Revision, Command: "task.create", Payload: core.Object{"title": "Calendar fixture", "patch": core.Object{"estimateMinutes": float64(100)}}})
	if err != nil {
		t.Fatal(err)
	}
	payload := core.Object{"taskId": calTask.Result["taskId"], "startAt": "2026-10-06T08:00:00Z", "zone": "Europe/Moscow", "minutes": 50}
	status, proposal := request("POST", "/api/v2/calendar/proposal", payload, nil)
	if status != 200 || len(proposal["blocks"].([]any)) != 3 {
		t.Fatal("calendar proposal", status, proposal)
	}
	cc := core.Command{OperationID: core.ID(), ExpectedRevision: calTask.CommittedRevision, Command: "workblocks.create", Payload: payload}
	status, cr := request("POST", "/api/v2/workspace/commands", cc, nil)
	if status != 200 {
		t.Fatal("calendar commit", cr)
	}
	status, replayCalendar := request("POST", "/api/v2/workspace/commands", cc, nil)
	if status != 200 || replayCalendar["committedRevision"] != cr["committedRevision"] {
		t.Fatal("calendar replay")
	}
	savedCal, _ := store.Read(ctx)
	if len(savedCal.WorkBlocks) != 3 {
		t.Fatal("duplicate blocks")
	}
	// A source update invalidates the proposal and a fresh proposal detects the meeting.
	external := queuedBatch{DeviceID: "unused", Batch: Batch{SchemaVersion: 1, BatchID: core.ID(), Source: "calendar", Sequence: 1, ObservedAt: time.Now(), OK: true, Data: model.SourceData{CalendarFrom: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), CalendarTo: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), Calendar: []model.CalendarEvent{{ID: "event", Title: "Meeting", Start: time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)}}}}}
	// Use the actual ingestion/worker path, preserving source ownership.
	calHeaders := map[string]string{"Authorization": headers["Authorization"], "Idempotency-Key": external.Batch.BatchID}
	status, resultCal := request("POST", "/api/v2/integrations/batches", external.Batch, calHeaders)
	if status != 202 {
		t.Fatal("calendar import", status, resultCal)
	}
	for deadline := time.Now().Add(10 * time.Second); ; {
		state, _ := store.Read(ctx)
		if len(state.CalendarSources) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("calendar worker timeout")
		}
		time.Sleep(50 * time.Millisecond)
	}
	payload["startAt"] = "2026-10-06T09:00:00Z"
	status, _ = request("POST", "/api/v2/calendar/proposal", payload, nil)
	if status != 422 {
		t.Fatal("external conflict not detected", status)
	}
	freshCal, _ := store.Read(ctx)
	for bid := range freshCal.WorkBlocks {
		_, err = store.Execute(ctx, core.Command{OperationID: core.ID(), ExpectedRevision: freshCal.Revision, Command: "workblocks.delete", Payload: core.Object{"blockId": bid}})
		if err != nil {
			t.Fatal(err)
		}
		freshCal, _ = store.Read(ctx)
	}
	if freshCal.Tasks[calTask.Result["taskId"].(string)] == nil || len(freshCal.Plans) != len(savedCal.Plans) {
		t.Fatal("block delete changed task or plan")
	}
	// Exercise actual MinIO bytes and private ownership metadata.
	fileBytes := []byte("Spotter integration object\n")
	req, _ := http.NewRequest("POST", srv.URL+"/api/v2/files", bytes.NewReader(fileBytes))
	req.Header.Set("X-Spotter-Request", "workspace")
	req.Header.Set("X-File-Name", "test.txt")
	req.Header.Set("X-Content-SHA256", hash(fileBytes))
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var uploaded map[string]any
	_ = json.NewDecoder(res.Body).Decode(&uploaded)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("upload", uploaded)
	}
	fileID := uploaded["fileId"].(string)
	defer files.Client.RemoveObject(context.Background(), files.Bucket, fileID, minio.RemoveObjectOptions{})
	res, err = client.Get(srv.URL + "/api/v2/files/" + fileID)
	if err != nil {
		t.Fatal(err)
	}
	downloaded, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !bytes.Equal(downloaded, fileBytes) {
		t.Fatal("S3 mismatch")
	}
	_, _ = request("POST", "/api/v2/devices/"+device["deviceId"].(string)+"/revoke", core.Object{}, nil)
	status, _ = request("GET", "/api/v2/integrations/batches/"+batch.BatchID, nil, headers)
	if status != 401 {
		t.Fatal("revoked device", status)
	}
	_, _ = request("POST", "/api/v2/auth/logout", core.Object{}, nil)
	status, _ = request("GET", "/api/v2/workspace", nil, nil)
	if status != 401 {
		t.Fatal("logout", status)
	}
	u, _ := url.Parse(srv.URL)
	if len(jar.Cookies(u)) != 0 {
		t.Fatal("session cookie retained")
	}
	t.Log("PASS: auth, origin, atomic commands, replay, concurrent revisions, immutable reads, real JetStream worker, candidate dedup, MinIO checksum, device revoke and logout")
}
