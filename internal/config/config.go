package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Refresh   RefreshConfig   `yaml:"refresh"`
	DailyPlan DailyPlanConfig `yaml:"daily_plan"`
	OpenAI    OpenAIConfig    `yaml:"openai"`
	Mail      MailConfig      `yaml:"mail"`
	Notes     NotesConfig     `yaml:"notes"`
	Storage   StorageConfig   `yaml:"storage"`
	Scripts   ScriptsConfig   `yaml:"scripts"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type RefreshConfig struct {
	IntervalSeconds int `yaml:"interval_seconds"`
}

type DailyPlanConfig struct {
	Enabled bool   `yaml:"enabled"`
	Time    string `yaml:"time"`
}

type OpenAIConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Model          string `yaml:"model"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type MailConfig struct {
	Limit int `yaml:"limit"`
}

type NotesConfig struct {
	Folder string `yaml:"folder"`
}

type StorageConfig struct {
	File string `yaml:"file"`
}

type ScriptsConfig struct {
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	Calendar       string `yaml:"calendar"`
	Reminders      string `yaml:"reminders"`
	Mail           string `yaml:"mail"`
	Notes          string `yaml:"notes"`
}

func Load(path string) (Config, error) {
	cfg := Default()
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := parseYAML(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	return cfg, nil
}

func Default() Config {
	cfg := Config{
		Server:    ServerConfig{Host: "127.0.0.1", Port: 8080},
		Refresh:   RefreshConfig{IntervalSeconds: 300},
		DailyPlan: DailyPlanConfig{Enabled: true, Time: "07:30"},
		OpenAI:    OpenAIConfig{Enabled: false, Model: "gpt-5.2", TimeoutSeconds: 30},
		Mail:      MailConfig{Limit: 20},
		Notes:     NotesConfig{Folder: "Notes"},
		Storage:   StorageConfig{File: "./data/state.json"},
		Scripts: ScriptsConfig{
			TimeoutSeconds: 30,
			Calendar:       "./scripts/calendar_export.scpt",
			Reminders:      "./scripts/reminders_export.scpt",
			Mail:           "./scripts/mail_inbox.scpt",
			Notes:          "./scripts/notes_export.scpt",
		},
	}
	return cfg
}

func (c *Config) applyDefaults() {
	def := Default()
	if c.Server.Host == "" {
		c.Server.Host = def.Server.Host
	}
	if c.Server.Port == 0 {
		c.Server.Port = def.Server.Port
	}
	if c.Refresh.IntervalSeconds <= 0 {
		c.Refresh.IntervalSeconds = def.Refresh.IntervalSeconds
	}
	if c.DailyPlan.Time == "" {
		c.DailyPlan.Time = def.DailyPlan.Time
	}
	if c.OpenAI.Model == "" {
		c.OpenAI.Model = def.OpenAI.Model
	}
	if c.OpenAI.TimeoutSeconds <= 0 {
		c.OpenAI.TimeoutSeconds = def.OpenAI.TimeoutSeconds
	}
	if c.Mail.Limit <= 0 {
		c.Mail.Limit = def.Mail.Limit
	}
	if c.Notes.Folder == "" {
		c.Notes.Folder = def.Notes.Folder
	}
	if c.Storage.File == "" {
		c.Storage.File = def.Storage.File
	}
	if c.Scripts.TimeoutSeconds <= 0 {
		c.Scripts.TimeoutSeconds = def.Scripts.TimeoutSeconds
	}
	if c.Scripts.Calendar == "" {
		c.Scripts.Calendar = def.Scripts.Calendar
	}
	if c.Scripts.Reminders == "" {
		c.Scripts.Reminders = def.Scripts.Reminders
	}
	if c.Scripts.Mail == "" {
		c.Scripts.Mail = def.Scripts.Mail
	}
	if c.Scripts.Notes == "" {
		c.Scripts.Notes = def.Scripts.Notes
	}
}

func (c Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c Config) RefreshInterval() time.Duration {
	return time.Duration(c.Refresh.IntervalSeconds) * time.Second
}

func (c Config) ScriptTimeout() time.Duration {
	return time.Duration(c.Scripts.TimeoutSeconds) * time.Second
}

func (c Config) OpenAITimeout() time.Duration {
	return time.Duration(c.OpenAI.TimeoutSeconds) * time.Second
}

func parseYAML(raw []byte, cfg *Config) error {
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	section := ""
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":") {
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if section == "" {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid line %q", line)
		}
		key := strings.TrimSpace(parts[0])
		value := cleanValue(parts[1])
		if err := setValue(cfg, section, key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"`)
	value = strings.Trim(value, `'`)
	return value
}

func setValue(cfg *Config, section, key, value string) error {
	switch section {
	case "server":
		switch key {
		case "host":
			cfg.Server.Host = value
		case "port":
			port, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid server.port: %w", err)
			}
			cfg.Server.Port = port
		}
	case "refresh":
		if key == "interval_seconds" {
			seconds, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid refresh.interval_seconds: %w", err)
			}
			cfg.Refresh.IntervalSeconds = seconds
		}
	case "daily_plan":
		switch key {
		case "enabled":
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid daily_plan.enabled: %w", err)
			}
			cfg.DailyPlan.Enabled = enabled
		case "time":
			cfg.DailyPlan.Time = value
		}
	case "openai":
		switch key {
		case "enabled":
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid openai.enabled: %w", err)
			}
			cfg.OpenAI.Enabled = enabled
		case "model":
			cfg.OpenAI.Model = value
		case "timeout_seconds":
			seconds, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid openai.timeout_seconds: %w", err)
			}
			cfg.OpenAI.TimeoutSeconds = seconds
		}
	case "mail":
		if key == "limit" {
			limit, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid mail.limit: %w", err)
			}
			cfg.Mail.Limit = limit
		}
	case "notes":
		if key == "folder" {
			cfg.Notes.Folder = value
		}
	case "storage":
		if key == "file" {
			cfg.Storage.File = value
		}
	case "scripts":
		switch key {
		case "timeout_seconds":
			seconds, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid scripts.timeout_seconds: %w", err)
			}
			cfg.Scripts.TimeoutSeconds = seconds
		case "calendar":
			cfg.Scripts.Calendar = value
		case "reminders":
			cfg.Scripts.Reminders = value
		case "mail":
			cfg.Scripts.Mail = value
		case "notes":
			cfg.Scripts.Notes = value
		}
	}
	return nil
}
