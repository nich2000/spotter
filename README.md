# Personal Spotter

Personal Spotter — локальный персональный ассистент для macOS, который агрегирует данные из Calendar, Reminders, Mail и Notes, формирует ежедневную сводку и отображает актуальную информацию через локальный веб-интерфейс в режиме реального времени.

The server runs locally and reads macOS sources through AppleScript without screenshots or OCR. Optional recommendations use the configured model endpoint: the checked-in configuration targets local Ollama; selecting a hosted endpoint sends source content to that service. Workspace decisions and sleep aggregates remain separate from model inputs.

## Requirements

- macOS with Calendar, Reminders, Mail and Notes.
- Go 1.22 or newer.
- `osascript`, available on macOS by default.
- Node.js 18 or newer for frontend regression tests only; no npm dependencies are required.

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

Порт можно переопределить при запуске:

```bash
go run ./cmd/spotter -p 8081
# или
go run ./cmd/spotter --port 8081
```

`-p` и `--port` задают HTTP-порт (1–65535) с приоритетом над `server.port`
в конфигурации. Без параметра используется конфигурация. Если параметры повторены,
действует последнее значение. Для примера выше откройте http://127.0.0.1:8081.


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
  interval_seconds: 7200
daily_plan:
  enabled: true
  time: "07:30"
openai:
  enabled: true
  # Ollama ignores the API key value, but the client requires OPENAI_API_KEY to be set.
  base_url: "http://localhost:11434/v1"
  model: "qwen3:4b"
  timeout_seconds: 60
mail:
  limit: 20
notes:
  folder: "Notes"
storage:
  file: "./data/state.json"
  audit_file: "./data/refresh_log.jsonl"
scripts:
  timeout_seconds: 30
```

Only the configured Notes folder is read. The default folder is `Notes`.

To run local recommendations through Ollama with `qwen3:4b`:

```bash
brew install ollama
ollama serve
```

In another terminal:

```bash
ollama pull qwen3:4b
ollama run qwen3:4b "Ответь одним предложением: модель работает?"
export OPENAI_API_KEY="ollama"
go run ./cmd/spotter
```

The repository includes [.env.example](.env.example) with the non-secret `OPENAI_API_KEY="ollama"` placeholder. For Ollama, any non-empty value is enough. For the hosted API, set `openai.base_url: "https://api.openai.com/v1"`, choose a hosted model, and provide a real API key through the environment. If the key is missing, Personal Spotter uses the local rule-based plan. If a model call fails, the dashboard reports that recommendations are unavailable; the failure is recorded in logs and audit.

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
internal/planner     model source summaries and rule-based fallback
internal/workspace   persistent tasks, focus schedule and sleep bridge
internal/audit       append-only diagnostics
tests                frontend regression tests (Node.js)
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
- `GET /api/workspace?date=YYYY-MM-DD` - saved decisions, candidates and proposed schedule.
- `POST /api/workspace?date=YYYY-MM-DD` - versioned workspace commands (requires `X-Spotter-Request: workspace`).
- `GET /api/focus.ics?date=YYYY-MM-DD` - export proposed focus blocks.

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
- Spotter recommendations are optional and use the configured model API when `openai.enabled` is true and `OPENAI_API_KEY` is set.
- The config parser supports the simple nested YAML shape used by `config.yaml`.
- No write operations are implemented for Calendar, Reminders, Mail or Notes.

## Диагностика обновлений и Ollama

Для сохранения консольного лога:

```bash
go run ./cmd/spotter 2>&1 | tee /tmp/spotter.log
```

Каждая операция получает `operation_id`. По нему связаны этапы сбора, запрос модели,
итог обновления и запись `operationId` в `storage.audit_file` (по умолчанию
`./data/refresh_log.jsonl`). `trigger` показывает источник запуска: `startup`,
`interval`, `manual_http` или `daily_schedule`. Одновременные операции выполняются
последовательно, время ожидания видно в `queue_ms`.

- `collector started/completed/failed`: источник, длительность в миллисекундах,
  количество событий, задач, писем и заметок; при ошибке — исходная причина.
  Нулевое количество при успешном сборе отличается от ошибки источника.
- `llm request started`: выбранный backend, модель, host/path и объём запроса.
- `planner completed`: `status`, `stage`, `http_status`, `duration_ms`,
  `done_reason`, число входных/выходных токенов, `load_ms` и `eval_ms`.
  Метрики токенов и времени модели доступны для Ollama, если сервер их возвращает;
  нули при ранней ошибке не означают успешную генерацию без затрат.
- `audit saved`, `state saved`, `state published`: подтверждение записи аудита,
  состояния и публикации в SSE broker. Публикация не подтверждает получение браузером.
- `refresh completed` и `scheduled plan completed`: `ok` либо `degraded`,
  количество ошибок и общая длительность выполнения (без ожидания очереди).

В JSONL сохраняются данные источников, длительности, полный JSON запроса модели
без заголовка Authorization, ответ сервера `responseBody` (не более 2 MiB + 1 байт),
выделенный текст ответа, итоговый план и ошибка. Там есть личные данные из источников;
перед передачей файла для анализа удалите ненужное личное содержимое.
Консольный лог не печатает prompt и текст ответа модели. Для анализа конкретного
сбоя достаточно прислать строки с одним `operation_id` и соответствующую JSONL-запись.
Если запись аудита не удалась, причина будет в `save refresh audit failed`.

Примеры интерпретации:

- `stage=request` и `context deadline exceeded`: истёк таймаут обращения к модели;
  `connection refused`: соединение с сервером не установлено.
- `stage=http_status`, `http_status=404`: сервер ответил ошибкой; точное сообщение
  смотрите в `model.responseBody` аудита (например, отсутствующая модель).
- `stage=generation`, `done_reason=length`: достигнут лимит `num_predict=2000`;
  ответ считается незавершённым, даже если его JSON удалось бы разобрать.
- `stage=validate_plan`: ответ получен, но не соответствует контракту плана.
  Обязательны непустой summary и массивы blocks/risks/focus; каждый массив содержит
  не более 8 непустых, неповторяющихся пунктов. Пустые массивы допустимы.
- Calendar `Application isn’t running (-600)`: ошибка macOS/Calendar при сборе,
  а не ошибка Ollama. Остальные источники продолжают обрабатываться.

Ограничение размера массивов также передаётся Ollama в JSON schema и инструкции.
Это ограничивает повторяющиеся длинные ответы, но не гарантирует фактическую
точность рекомендаций. Поля метрик соответствуют [Ollama Chat API](https://docs.ollama.com/api/chat).

## Выжимки источников и фокус дня

При включённой модели сбор каждого источника запускает отдельную суммаризацию
через настроенный API (для qwen3 — Ollama). Запрос содержит только данные этого
источника. Успешный пустой источник также суммаризируется; сбой сбора помечается
`skipped`, сбой суммаризации — `failed`. После завершения всех источников модель
получает только выжимки и статусы и формирует общий фокус дня. Если ни одной успешной
выжимки нет, итоговая генерация не запускается. При четырёх успешных источниках
выполняется пять запросов модели; таймаут применяется отдельно к каждому запросу.

Выжимки доступны в `sourceSummaries` API состояния и в карточках источников.
Исходные записи сохраняются для просмотра, но не передаются повторно в итоговый
запрос. JSONL содержит `sources[].summary` и `sources[].model` для каждого источника,
а `model` верхнего уровня — трассу общей суммаризации. В логах модельных вызовов
`source` различает источники (пустое значение — итоговая генерация).
Ежедневная генерация использует сохранённые выжимки с их временными метками;
старое состояние без выжимок сначала полностью обновляется. При отключённой модели
сохраняется прежний локальный алгоритм без модельных выжимок.

### Запуск Calendar

Сборщик запускает Calendar в фоне через LaunchServices (`open -g -b com.apple.iCal`),
затем проверяет доступность календарей. Прежняя AppleScript-команда `launch` могла
сама завершаться ошибкой `-600`, до начала чтения данных. При `-600` проверка
готовности повторяется до 20 раз с паузой 0,5 с; остальные ошибки, включая запрет
Automation, возвращаются сразу. Общий лимит выполнения задаёт `scripts.timeout_seconds`.
Calendar остаётся открытым после чтения; события не изменяются.

## Personal workflow: GTD, Kanban, MIT and focus blocks

The dashboard now has a persistent, deterministic workspace at `/api/workspace`.
User decisions are stored atomically in `<storage.file>.workspace.json` with mode
0600, independently from source refreshes and LLM summaries. Invalid/corrupt saved
workspace data stops startup rather than silently replacing it. Back up this file
along with the regular state file; it includes sleep aggregates after connection.

Daily workflow:

1. Capture ideas in Inbox without interrupting work. A project is optional for
   one-off actions. Refine the result and estimate only when processing the task.
2. Select relevant suggestions from Calendar, Reminders, Mail and Notes, or skip
   them. Skips persist and can be restored together. Sources are never modified.
   Failed-pipeline messages with the same sender, repository and commit are grouped;
   subjects, senders and dates remain available in the suggestion details. Existing
   imports made before grouping are respected. Other emails use a neutral “review”
   action rather than automatically implying a reply.
3. Move tasks through deferred → ready → doing → waiting / done. Server-enforced
   limits: ready 15; doing and waiting together 3. Existing workspaces above the WIP
   limit remain editable, but cannot increase WIP until it falls below the limit.
   Enter a reason and follow-up date when moving a task into waiting. Existing
   waiting tasks without metadata remain readable and appear in review attention.
4. Choose one main result directly on the day screen, with up to two additional
   results (three total including completed tasks). The main result is scheduled
   first, regardless of task creation order. Replacing it leaves the previous
   result as an additional task; a full day must be reduced before adding more work.
   A task belongs to one planned date; choosing another date explicitly moves it.
   Existing dated tasks remain additional until a main result is chosen.
5. Save a return note in the task: current progress, code/log references and next
   experiment. Mark completion directly from its card. At most five active weekly
   priorities remain supported; two primary projects are a suggested starting point,
   not a new hard limit. Deferred-only projects are not treated as active.
6. Review available time, budget and selected work. Unavailable calendar data is
   shown as unknown, never as zero capacity. Settings retain existing values:
   work window, IANA timezone, 20–30% reserve, 30–120 minute focus block,
   10–20 minute break, personal sleep target. Defaults remain Moscow, 09:30–18:00,
   25%, 90/15 minutes, 8-hour sleep target. Health and iPhone setup are collapsed.
7. Download `.ics` and import into Calendar if desired. Blocks are explicitly
   proposed, not booked. Export does not confirm import. Refresh Calendar after
   importing and review before another export; there is no two-way synchronization.
8. Start a focus timer for today's selected task in an available block. It persists
   in browser local storage. Timer completion does not complete the task.
9. Use “Plan tomorrow” to open tomorrow's selection. The review screen surfaces
   overdue planned tasks, waiting tasks due for follow-up and tasks in progress
   untouched for seven days. Save decisions without mandatory ritual checkboxes.
   Review notes share the existing latest-review record; this is not a daily journal.

New optional task fields (`mainDate`, `resume`, `waitingOn`, `checkDate`) and the
workspace `dismissed` list are persisted with existing optimistic-version checks.
Old workspace files require no migration and user settings are not reset.

Scheduling uses the collected calendar's explicit `[calendarFrom, calendarTo)`
range (currently today plus seven days), requires a successful snapshot no older
than 24 hours, merges overlapping busy intervals, excludes past time and leaves
capacity unassigned. Focus and breaks together fit within the non-reserved share
of remaining free time. Long estimates split into blocks; incomplete estimates
produce explicit warnings. Meetings that began before the collection window are
included if they overlap it. Calendar collection and configured work timezone
should match the actual local calendar; the existing AppleScript uses +03:00.

Task mutations use optimistic versions: concurrent edits return HTTP 409 instead
of overwriting another change. After an editor conflict, close and reopen the card to load the latest task; retries from the stale editor retain its original version and cannot overwrite another tab. The unsaved text stays in the form until it is closed. Writes
require `X-Spotter-Request: workspace` and a same-host Origin when provided. The
application retains its local-server deployment model; do not expose it publicly.
`GET /api/focus.ics?date=YYYY-MM-DD` exports only proposed focus blocks.

Source deduplication uses stable Mail IDs where available and content fingerprints
for the current Calendar/Reminders/Notes contracts, which lack native IDs. A renamed
source item can therefore appear as a new candidate; identical titles in the same
list/folder are treated as one. Imported tasks retain source attribution but do not
track later edits or completion in Apple apps. The older LLM output remains under
“Обзор исходных данных и выжимка модели”; it cannot bypass workspace limits.

## Automatic iPhone health bridge

See the local setup page at `/health-setup.html`. Create an iPhone Shortcut to export
**only actual asleep intervals** (Asleep/Core/Deep/REM, not InBed/Awake) and the export
time into `iCloud Drive/Spotter/health.json`. Configure a daily personal automation
on the iPhone. Both devices need the same iCloud account and iCloud Drive enabled.
The phone-side Shortcut and Health permissions must be configured on the device;
this repository does not claim they are already installed or authorized.

Spotter watches the file every minute while running. Default Mac path:
`~/Library/Mobile Documents/com~apple~CloudDocs/Spotter/health.json`.
Override with `SPOTTER_HEALTH_FILE` in `.env` or process environment, then restart.
The desktop server does not need a public or LAN health-ingestion endpoint.
Docker needs an explicit read-only host folder mount and the corresponding variable.

JSON contract (ISO 8601 timestamps with timezone, example data only):

```json
{
  "measuredAt": "2026-09-14T08:30:00+03:00",
  "sleep": [
    {"start": "2026-09-14T00:10:00+03:00", "end": "2026-09-14T07:30:00+03:00"}
  ]
}
```

The bridge limits input to 2 MB / 5000 intervals, validates dates, rejects future
measurements and overlapping-file-write fragments, ignores older exports, clips
to the 24-hour window before export, and unions sleep intervals to avoid double
counting. Use one trusted device/source in the Shortcut. Missing samples mean
**unknown**, never zero. The full input is not stored in workspace or audit;
only sleep hours and timestamps are retained locally. Health is not added to
`AppState` sent to the LLM and never enters source-summary or model audit payloads.
The user-selected iCloud transport itself is separate from local processing.

Fresh sleep below the user's target selects shorter focus blocks (up to 45 minutes).
After 36 hours since last sleep, automatic adjustment stops. Manual energy choices
are dated; tomorrow does not inherit today's tiredness or today's measurements.
This is a workload preference, not a medical readiness score. This version does
not import heart rate, HRV, steps or workouts. iOS authorization/device lock and
cloud delivery can delay updates; inspect export timestamps and bridge status.

## Validation

```bash
go test -race -count=1 ./...
go vet ./...
go build -o .cache/spotter ./cmd/spotter
node --check web/app.js
node --check web/workspace.js
node --test tests/*.test.cjs
git diff --check
```

Go tests use temporary files and local HTTP test servers; they do not read Apple
apps, call the configured model endpoint or start the production server. Allow
local test listeners if a sandbox blocks them. Frontend tests use Node's built-in
runner and a simulated DOM/API to verify conflict retries, project activity and
unknown calendar capacity. Visual browser checks remain a separate validation.

Coverage includes task limits and Unicode text lengths, persistence, main-result
ordering, waiting follow-ups, source grouping/skip/restore, meeting conflicts,
calendar freshness/coverage, reserve, model failure diagnostics, sleep interval
union and bridge recovery. Real Apple Automation permissions, model quality and
iPhone delivery require separate runtime checks.
