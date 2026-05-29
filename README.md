# Personal Spotter

Personal Spotter — локальный персональный ассистент для macOS, который агрегирует данные из Calendar, Reminders, Mail и Notes, формирует ежедневную сводку и отображает актуальную информацию через локальный веб-интерфейс в режиме реального времени.

The MVP runs only on the local computer, does not call external APIs, does not use screenshots or OCR, and does not control the GUI.

## Requirements

- macOS with Calendar, Reminders, Mail and Notes.
- Go 1.22 or newer.
- `osascript`, available on macOS by default.

## Install

```bash
git clone <repo-url>
cd spotter
go build ./cmd/spotter
```

## Run

```bash
go run ./cmd/spotter
```

Open:

```text
http://localhost:8080
```

The server listens on `127.0.0.1` by default.

## Docker

Docker is useful for checking the web server, SSE stream, planner and error handling. It cannot collect real macOS Calendar, Reminders, Mail or Notes data because containers do not have access to the host macOS Automation APIs or `osascript`.

```bash
docker build -t personal-spotter:local .
docker run --rm --name personal-spotter -p 127.0.0.1:8080:8080 personal-spotter:local
```

Open:

```text
http://localhost:8080
```

## Configuration

Edit `config.yaml`:

```yaml
server:
  host: "127.0.0.1"
  port: 8080
refresh:
  interval_seconds: 300
daily_plan:
  enabled: true
  time: "07:30"
openai:
  enabled: true
  # Set the API key through the OPENAI_API_KEY environment variable.
  model: "gpt-5.2"
  timeout_seconds: 30
mail:
  limit: 20
notes:
  folder: "Notes"
storage:
  file: "./data/state.json"
scripts:
  timeout_seconds: 30
```

Only the configured Notes folder is read. The default folder is `Notes`.

To ask OpenAI for recommendations, set `openai.enabled: true` and provide an API key through the environment:

```bash
export OPENAI_API_KEY=""
export OPENAI_API_KEY="..."
go run ./cmd/spotter
```

The repository includes [.env.example](/Users/nich/develop/spotter/.env.example) with an empty `OPENAI_API_KEY=` placeholder. The API key should not be stored in `config.yaml`. If the key is missing or the API call fails, Personal Spotter falls back to the local rule-based plan.

## macOS Permissions

On first run, macOS may ask for Automation permissions to access:

- Mail
- Calendar
- Reminders
- Notes

If a source returns a permission error, enable access in:

```text
System Settings -> Privacy & Security -> Automation
```

Some macOS versions may also require:

```text
System Settings -> Privacy & Security -> Full Disk Access
```

The app keeps running when a source fails. The dashboard shows the source as failed and continues updating the remaining sources.

## Project Structure

```text
cmd/spotter        entrypoint
internal/app         state orchestration
internal/config      YAML config loader
internal/server      HTTP routes
internal/sse         Server-Sent Events broker
internal/scheduler   periodic refresh and daily plan scheduling
internal/collectors  Calendar, Reminders, Mail and Notes collectors
internal/planner     rule-based daily planner
internal/storage     JSON file storage
internal/model       shared state models
scripts              AppleScript sources
web                  HTML/CSS/JS dashboard
deploy               launchd example
```

## HTTP API

- `GET /` - dashboard.
- `GET /events` - SSE stream with `event: update`.
- `GET /api/state` - current state as JSON.
- `POST /api/refresh` - manual refresh.

## Add A New Source

1. Add a package under `internal/collectors/<source>`.
2. Implement:

```go
type Collector interface {
    Name() string
    Collect(ctx context.Context) (model.SourceData, error)
}
```

3. Add the collector to `cmd/spotter/main.go`.
4. Extend `model.SourceData` and `model.AppState` if the source needs new fields.
5. Update the web UI to render the new data.

Collector errors should be returned to the caller. The app records them in `SourceStatus` and continues refreshing other sources.

## launchd Autostart

Build and place the binary and config where the plist expects them, or edit `deploy/com.personal-spotter.plist` paths:

```bash
go build -o /usr/local/bin/spotter ./cmd/spotter
sudo mkdir -p /usr/local/etc/spotter
sudo cp config.yaml /usr/local/etc/spotter/config.yaml
cp deploy/com.personal-spotter.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.personal-spotter.plist
```

Unload:

```bash
launchctl unload ~/Library/LaunchAgents/com.personal-spotter.plist
```

## Known Limitations

- AppleScript access depends on macOS permissions and app availability.
- Mail previews are intentionally empty in the MVP to avoid reading full message bodies.
- The Mail dashboard shows unread messages only.
- OpenAI recommendations are optional and use the Responses API when `OPENAI_API_KEY` is set.
- The config parser supports the simple nested YAML shape used by `config.yaml`.
- No write operations are implemented for Calendar, Reminders, Mail or Notes.
