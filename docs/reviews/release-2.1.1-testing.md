# Testing — 2.1.1

Дата: 2026-10-05. Проверены все текущие изменения после `release-2.1.1-review.md`, включая исправления ревью. Подтвержденных новых ошибок в тестируемых сценариях не обнаружено; код на этапе тестирования не изменялся. Коммиты и обновление рабочего Compose не выполнялись.

## Результаты

| Проверка | Команда | Результат |
| --- | --- | --- |
| Все Go-пакеты, race и coverage | `go test -race -coverprofile=/private/tmp/spotter-2.1.1-coverage.out ./... -count=1` | PASS; итоговое statement coverage 80.8%; core 92.0%, platform 67.1%, agent 81.0% |
| Статический анализ | `go vet ./...` | PASS |
| Legacy frontend regression | `node --test tests/*.test.cjs` | PASS, 3/3 |
| React frontend, coverage thresholds | `cd frontend && npm test` | PASS, 12 файлов, 58 тестов; statements 90.54%, branches 85.84%, functions 89.90%, lines 92.08%; все пороги 80% пройдены |
| TypeScript и production bundle | `cd frontend && npm run build` | PASS, 60 modules, 1m29s; JS 306.05 kB, gzip 95.99 kB |
| Docker production image | `docker build -t spotter:testing-2.1.1 .` | PASS; Node 22.19.0 Alpine и Go 1.26.3 Alpine; внутри сборки `npm ci` сообщила 0 vulnerabilities; Vite 2m8s |
| PostgreSQL / JetStream / MinIO integration | Команда ниже | PASS, `TestIntegration`, 1.54s |
| AppleScript syntax | `osacompile -o /private/tmp/spotter-<source>-2.1.1.scpt scripts/<script>.scpt` | PASS для calendar_export, mail_inbox, notes_export, reminders_export, вне sandbox; скрипты не выполнялись |
| Whitespace | `git diff --check` | PASS |

Интеграция использовала отдельный проект и тестовые значения; рабочие данные и порты не использовались:

```sh
POSTGRES_PASSWORD=spotter-test-only NATS_PASSWORD=spotter-test-only \
MINIO_ROOT_USER=spottertest MINIO_ROOT_PASSWORD=spotter-test-only \
docker compose -p spotter-release-211-test -f compose.yaml -f compose.test.yaml \
  --profile test run --rm test
```

Проверены первичная и повторная миграция, authentication/setup/logout, проверка Origin, атомарность и rollback команд, operation replay, конкурентные workspace revisions, immutable snapshots, реальный JetStream worker, дедупликация кандидатов, checksum файлов MinIO и отзыв helper device. После успешного запуска созданный тестовый проект удален командой с теми же env и файлами: `docker compose -p spotter-release-211-test -f compose.yaml -f compose.test.yaml --profile test down -v`. Удалены только его контейнеры, сеть и тома; рабочий Compose не изменялся.

Исправления ревью имеют отдельные regression tests и были дважды проверены: сначала соответствующими unit/race/frontend тестами при ревью, затем полными Go race и frontend suites на этом этапе. Сборка frontend подтверждена независимо локальным npm build и Docker build. Ошибок compilation/runtime в этих проверках нет.

## Ограничения доказательства

- Go Docker test image сообщает `go version go1.26.3 linux/arm64`, `CGO_ENABLED=0`, gcc отсутствует. Интеграция выполнялась без race согласно `compose.test.yaml`; race проверен отдельно для всех локальных Go-пакетов.
- Первая sandbox-компиляция Calendar сообщила `Expected end of line but found identifier` на `do shell script`. Повтор вне sandbox успешен для всех четырех скриптов: причина связана с доступностью AppleScript словарей в sandbox, а не подтвержденным дефектом исходника. Компиляция не подтверждает Apple Automation permissions, запуск/чтение реальных приложений и пользовательские данные.
- `docker run --rm spotter:testing-2.1.1 -version` завершилась exit 2: `flag provided but not defined: -version`. Новый server CLI имеет только `-role` и `-legacy`; номер 2.1.1 подтвержден в frontend package. Встроенная версия server binary не подтверждена, отдельный version command не добавлялся.
- `govulncheck` не установлен; Go vulnerability scan не выполнен. Сообщение npm audit относится к npm dependency install в Docker и не заменяет Go scan.
- Проверки не подтверждают Git publication, deployment, здоровье рабочего Compose после обновления или реальную доставку Apple данных. Эти действия относятся к последующему этапу выпуска.
