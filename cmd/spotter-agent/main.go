package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"spotter/internal/agent"
	"spotter/internal/collectors"
	"spotter/internal/collectors/calendar"
	"spotter/internal/collectors/mail"
	"spotter/internal/collectors/notes"
	"spotter/internal/collectors/reminders"
	"spotter/internal/collectors/script"
	"spotter/internal/config"
	"spotter/internal/core"
	"spotter/internal/platform"
	"spotter/internal/workspace"
)

func main() {
	server := flag.String("server", "http://localhost:18080", "server URL")
	pair := flag.String("pair", "", "one-use pairing code")
	once := flag.Bool("once", false, "collect and flush once")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *server, *pair, *once); err != nil {
		slog.Error("helper failed", "error", err)
		os.Exit(1)
	}
}
func keychain(ctx context.Context, service string) (string, error) {
	out, err := exec.CommandContext(ctx, "security", "find-generic-password", "-a", "spotter", "-s", service, "-w").Output()
	return strings.TrimSpace(string(out)), err
}
func saveKey(ctx context.Context, service, value string) error {
	return exec.CommandContext(ctx, "security", "add-generic-password", "-U", "-a", "spotter", "-s", service, "-w", value).Run()
}
func run(ctx context.Context, server, pair string, once bool) error {
	if runtime.GOOS != "darwin" {
		return errors.New("helper requires macOS")
	}
	u, err := url.Parse(server)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Path != "" {
		return errors.New("invalid server URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")) {
		return errors.New("HTTPS is required outside loopback")
	}
	service := "spotter-device-" + u.Host
	token, tokenErr := keychain(ctx, service)
	if pair != "" {
		name, _ := os.Hostname()
		raw, _ := json.Marshal(map[string]string{"code": pair, "name": name})
		req, err := http.NewRequestWithContext(ctx, "POST", server+"/api/v2/devices/enroll", bytes.NewReader(raw))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		client := http.Client{Timeout: 30 * time.Second}
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			var failure struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&failure)
			return fmt.Errorf("pairing at %s returned HTTP %d: %s — %s", server, res.StatusCode, failure.Error.Code, failure.Error.Message)
		}
		var data struct {
			DeviceID string `json:"deviceId"`
			Token    string `json:"token"`
		}
		if err = json.NewDecoder(res.Body).Decode(&data); err != nil {
			return err
		}
		token = data.Token
		if err = saveKey(ctx, service, token); err != nil {
			return err
		}
		tokenErr = nil
	}
	if tokenErr != nil || token == "" {
		return errors.New("pair this Mac using a code from Settings")
	}
	keyHex, err := keychain(ctx, service+"-queue")
	if err != nil {
		keyHex = core.ID() + core.ID()
		if err = saveKey(ctx, service+"-queue", keyHex); err != nil {
			return err
		}
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	queueDir := os.Getenv("SPOTTER_AGENT_DIR")
	if queueDir == "" {
		queueDir = filepath.Join(home, "Library", "Application Support", "Spotter", u.Host, "queue")
	}
	a := agent.Agent{URL: server, Token: token, Key: key, Dir: queueDir}
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return err
	}
	runner := script.Runner{Timeout: cfg.ScriptTimeout()}
	sources := []collectors.Collector{calendar.Collector{ScriptPath: cfg.Scripts.Calendar, Runner: runner}, reminders.Collector{ScriptPath: cfg.Scripts.Reminders, Runner: runner}, mail.Collector{ScriptPath: cfg.Scripts.Mail, Limit: cfg.Mail.Limit, Runner: runner}, notes.Collector{ScriptPath: cfg.Scripts.Notes, Folder: cfg.Notes.Folder, Runner: runner}}
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		if err = a.Flush(ctx); err != nil {
			slog.Warn("delivery pending", "error", err)
		}
		for _, source := range sources {
			data, collectErr := source.Collect(ctx)
			b := platform.Batch{SchemaVersion: 1, BatchID: core.ID(), Source: source.Name(), Sequence: time.Now().UnixMilli(), ObservedAt: time.Now().UTC(), OK: collectErr == nil, Data: data}
			if collectErr != nil {
				b.Error = "COLLECTION_FAILED"
				slog.Warn("source collection failed", "source", source.Name())
			}
			if err = a.Enqueue(b); err != nil {
				return err
			}
		}
		if err = a.Flush(ctx); err != nil {
			slog.Warn("delivery pending", "error", err)
		}
		if err = a.CollectHealth(ctx, workspace.DefaultHealthPath()); err != nil {
			slog.Warn("health bridge deferred", "error", err)
		}
		if once {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
