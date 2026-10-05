// Package agent provides an encrypted, durable outbound queue for the macOS helper.
package agent

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"spotter/internal/core"
	"spotter/internal/platform"
	"spotter/internal/workspace"
)

type Agent struct {
	URL, Token, Dir string
	Key             []byte
	Client          *http.Client
}

func (a *Agent) seal(raw []byte) ([]byte, error) {
	block, err := aes.NewCipher(a.Key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, raw, nil), nil
}
func (a *Agent) open(raw []byte) ([]byte, error) {
	block, err := aes.NewCipher(a.Key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, fmt.Errorf("truncated queue entry")
	}
	return gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
}
func (a *Agent) Enqueue(b platform.Batch) error {
	if err := platform.ValidateBatch(b); err != nil {
		return err
	}
	if strings.ContainsAny(b.BatchID, "/\\.") {
		return fmt.Errorf("unsafe batch id")
	}
	if err := os.MkdirAll(a.Dir, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(a.Dir)
	if err != nil {
		return err
	}
	var size int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	if size+int64(len(raw)) > 64<<20 {
		return fmt.Errorf("queue full: delivery required before further collection")
	}
	sealed, err := a.seal(raw)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(a.Dir, ".pending-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(sealed)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(a.Dir, b.BatchID+".batch"))
}
func (a *Agent) request(ctx context.Context, method, path string, raw []byte, id string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, method, a.URL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)
	req.Header.Set("Content-Type", "application/json")
	if id != "" {
		req.Header.Set("Idempotency-Key", id)
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("server returned HTTP %d", res.StatusCode)
	}
	var result map[string]any
	err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&result)
	return result, err
}
func (a *Agent) Flush(ctx context.Context) error {
	entries, err := os.ReadDir(a.Dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".batch") {
			continue
		}
		path := filepath.Join(a.Dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		plain, err := a.open(raw)
		if err != nil {
			return err
		}
		var b platform.Batch
		if err = json.Unmarshal(plain, &b); err != nil {
			return err
		}
		res, err := a.request(ctx, "POST", "/api/v2/integrations/batches", plain, b.BatchID)
		if err != nil {
			return err
		}
		state, _ := res["state"].(string)
		if state != "applied" && state != "rejected" {
			res, err = a.request(ctx, "GET", "/api/v2/integrations/batches/"+b.BatchID, nil, "")
			if err != nil {
				return err
			}
			state, _ = res["state"].(string)
		}
		if state == "rejected" {
			return fmt.Errorf("batch rejected; retained for inspection: %s", b.BatchID)
		}
		if state == "applied" {
			if err = os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

// CollectHealth reads the selected iCloud file only after an explicit current server consent.
func (a *Agent) CollectHealth(ctx context.Context, path string) error {
	consent, err := a.request(ctx, "GET", "/api/v2/integrations/consents", nil, "")
	if err != nil {
		return err
	}
	if consent["enabled"] != true {
		return nil
	}
	generation, ok := consent["generation"].(float64)
	if !ok {
		return fmt.Errorf("invalid consent generation")
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	health, err := workspace.ParseHealth(f, time.Now())
	if err != nil {
		return err
	}
	return a.Enqueue(platform.Batch{SchemaVersion: 1, BatchID: core.ID(), Source: "health", Sequence: time.Now().UnixMilli(), ObservedAt: time.Now(), OK: true, Health: &health, ConsentGeneration: int64(generation)})
}
