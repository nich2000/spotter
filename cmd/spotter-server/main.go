package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"spotter/internal/platform"
)

func main() {
	role := flag.String("role", "api", "api, worker or migrate")
	legacy := flag.String("legacy", "", "legacy workspace JSON path for import/dry-run")
	flag.Parse()
	if *legacy != "" {
		if err := migrateLegacy(*legacy, *role == "import"); err != nil {
			slog.Error("legacy migration failed", "error", err)
			os.Exit(1)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *role); err != nil {
		slog.Error("service failed", "error", err)
		os.Exit(1)
	}
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func run(ctx context.Context, role string) error {
	s, err := platform.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer s.DB.Close()
	if role == "migrate" {
		return s.Migrate(ctx)
	}
	if _, err = s.Read(ctx); err != nil {
		return err
	}
	b, err := platform.ConnectBroker(env("NATS_URL", "nats://nats:4222"))
	if err != nil {
		return err
	}
	defer b.Conn.Close()
	if err = b.Init(); err != nil {
		return err
	}
	if role == "worker" {
		return b.Run(ctx, s)
	}
	if role != "api" {
		return errors.New("unknown service role")
	}
	files, err := platform.OpenFiles(env("S3_ENDPOINT", "minio:9000"), os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"))
	if err != nil {
		return err
	}
	if err = files.Init(ctx); err != nil {
		return err
	}
	api := &platform.API{Store: s, Origin: env("SPOTTER_ORIGIN", "http://localhost:8080"), Assets: env("SPOTTER_ASSETS", "/app/web"), Broker: b, Files: files}
	server := &http.Server{Addr: env("SPOTTER_LISTEN", ":8080"), Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	slog.Info("spotter API started", "origin", api.Origin)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func migrateLegacy(path string, apply bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s, err := platform.Open(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer s.DB.Close()
	state, err := s.ImportLegacy(context.Background(), raw, !apply)
	if err != nil {
		return err
	}
	report := map[string]any{"tasks": len(state.Tasks), "projects": len(state.Projects), "sourceSHA256": state.Migration["sourceSHA256"], "applied": apply, "historyCompleteness": "partial"}
	b, _ := json.Marshal(report)
	fmt.Println(string(b))
	return nil
}
