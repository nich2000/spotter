package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"spotter/internal/app"
	"spotter/internal/audit"
	"spotter/internal/collectors"
	"spotter/internal/collectors/calendar"
	"spotter/internal/collectors/mail"
	"spotter/internal/collectors/notes"
	"spotter/internal/collectors/reminders"
	"spotter/internal/collectors/script"
	"spotter/internal/config"
	"spotter/internal/planner"
	"spotter/internal/scheduler"
	"spotter/internal/server"
	"spotter/internal/sse"
	"spotter/internal/storage"
	"spotter/internal/workspace"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	var port int
	registerPortFlags(flag.CommandLine, &port)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := config.LoadEnvFile(".env"); err != nil {
		logger.Error("load env file failed", "error", err)
		os.Exit(1)
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}
	if port != 0 {
		cfg.Server.Port = port
	}
	if cfg.Server.Host != "127.0.0.1" && cfg.Server.Host != "localhost" && os.Getenv("SPOTTER_DOCKER") != "1" {
		logger.Error("server host must be localhost-only", "host", cfg.Server.Host)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := script.Runner{Timeout: cfg.ScriptTimeout()}
	broker := sse.NewBroker(logger)
	dailyPlanner := planner.Planner(planner.RuleBased{})
	if cfg.OpenAI.Enabled {
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			logger.Warn("openai planner enabled but OPENAI_API_KEY is empty; using rule-based planner")
		} else {
			dailyPlanner = planner.OpenAI{
				APIKey:   apiKey,
				BaseURL:  cfg.OpenAI.BaseURL,
				Model:    cfg.OpenAI.Model,
				Timeout:  cfg.OpenAITimeout(),
				Fallback: planner.RuleBased{},
				Logger:   logger,
			}
		}
	}
	collectors := []collectors.Collector{
		calendar.Collector{ScriptPath: cfg.Scripts.Calendar, Runner: runner},
		reminders.Collector{ScriptPath: cfg.Scripts.Reminders, Runner: runner},
		mail.Collector{ScriptPath: cfg.Scripts.Mail, Limit: cfg.Mail.Limit, Runner: runner},
		notes.Collector{ScriptPath: cfg.Scripts.Notes, Folder: cfg.Notes.Folder, Runner: runner},
	}
	spotter := app.New(
		logger,
		collectors,
		dailyPlanner,
		storage.JSONStore{Path: cfg.Storage.File},
		&audit.JSONLStore{Path: cfg.Storage.AuditFile},
		broker,
	)
	spotter.Load(ctx)

	sched := scheduler.New(logger, cfg.RefreshInterval(), cfg.DailyPlan.Enabled, cfg.DailyPlan.Time, func(ctx context.Context) {
		spotter.Refresh(planner.WithTrigger(ctx, "interval"))
	}, func(ctx context.Context) {
		spotter.GeneratePlan(planner.WithTrigger(ctx, "daily_schedule"))
	})
	sched.Start(ctx)
	go func() {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			spotter.Refresh(planner.WithTrigger(ctx, "startup"))
		}
	}()

	work, err := workspace.New(cfg.Storage.File + ".workspace.json")
	if err != nil {
		logger.Error("load workspace failed", "error", err)
		os.Exit(1)
	}
	healthBridge := &workspace.Bridge{Path: workspace.DefaultHealthPath(), Service: work}
	go healthBridge.Run(ctx)

	httpServer := &http.Server{
		Addr:              cfg.Address(),
		Handler:           server.New(logger, spotter, broker).WithWorkspace(work, healthBridge).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Warn("server shutdown failed", "error", err)
		}
	}()

	logger.Info("spotter started", "address", "http://"+cfg.Address(), "planner", fmt.Sprintf("%T", dailyPlanner), "model", cfg.OpenAI.Model, "model_timeout", cfg.OpenAITimeout(), "script_timeout", cfg.ScriptTimeout(), "refresh_interval", cfg.RefreshInterval(), "audit_file", cfg.Storage.AuditFile, "state_file", cfg.Storage.File)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func registerPortFlags(flags *flag.FlagSet, port *int) {
	setPort := func(value string) error {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			return fmt.Errorf("HTTP port must be an integer between 1 and 65535")
		}
		*port = parsed
		return nil
	}
	flags.Func("p", "HTTP port (1-65535); overrides server.port in config", setPort)
	flags.Func("port", "HTTP port (1-65535); overrides server.port in config", setPort)
}
