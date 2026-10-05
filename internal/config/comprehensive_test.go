package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFullConfiguration(t *testing.T) {
	var c Config
	c.applyDefaults()
	if c.Address() == "" || c.RefreshInterval() <= 0 || c.ScriptTimeout() <= 0 || c.OpenAITimeout() <= 0 {
		t.Fatal(c)
	}
	raw := `server:
  host: localhost
  port: 8081
refresh:
  interval_seconds: 60
daily_plan:
  enabled: true
  time: '12:00'
openai:
  enabled: false
  model: example
  base_url: http://localhost:11434/v1
  timeout_seconds: 20
mail:
  limit: 25
notes:
  folder: Work
storage:
  file: state.json
  audit_file: audit.jsonl
scripts:
  timeout_seconds: 15
  calendar: calendar.scpt
  reminders: reminders.scpt
  mail: mail.scpt
  notes: notes.scpt
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if e := os.WriteFile(path, []byte(raw), 0600); e != nil {
		t.Fatal(e)
	}
	c, e := Load(path)
	if e != nil || c.Server.Port != 8081 || c.Notes.Folder != "Work" || c.Scripts.Notes != "notes.scpt" {
		t.Fatal(c, e)
	}
	for _, pair := range [][2]string{{"server", "port"}, {"refresh", "interval_seconds"}, {"daily_plan", "enabled"}, {"openai", "enabled"}, {"openai", "timeout_seconds"}, {"mail", "limit"}, {"scripts", "timeout_seconds"}} {
		if e = setValue(&c, pair[0], pair[1], "bad"); e == nil {
			t.Fatal(pair)
		}
	}
	if _, e = Load(path + "missing"); e == nil {
		t.Fatal("missing")
	}
}
