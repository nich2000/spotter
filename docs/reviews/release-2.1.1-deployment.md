# Публикация и локальное развёртывание — 2.1.1

2026-10-05, 21:10 Europe/Moscow.

- Релизный коммит: `e4f254ace3e540c6ec3620f1deed9b86e7572e9b`, 153 файла. Ревью, тестирование и документация выполнены последовательно отдельными субагентами. Локальные секреты и пользовательские данные исключены из Git.
- `git push --atomic origin main refs/tags/v2.1.1` завершён успешно. `git ls-remote` подтвердил main и peeled annotated tag на релизном коммите; объект тега `f0d4d6ecaa996630b2b74bc2033d6da47978dfe8`.
- `docker compose --env-file .env.docker up -d --build --wait` — exit 0. API/worker/MinIO пересозданы; PostgreSQL и NATS продолжили работу. Именованные тома не удалялись. Migrate — Exited (0); API healthy; worker, PostgreSQL, NATS и MinIO работают.
- `GET http://localhost:18080/readyz` — HTTP 200, `{"status":"ready"}`. `/calendar` выдаёт `index-Su3bOWDE.js` и `index-BXXS6bCX.css`; JS внутри рабочего API содержит версию 2.1.1.
- До обновления прошла изолированная PostgreSQL/NATS/MinIO интеграция. После обновления проверены HTTP, миграция и статусы; новый integration-прогон на рабочем стеке не выполнялся.

Существующий контейнер `spotter-nats-test-1` оставлен без изменений. Native helper не перезапускался, реальные Apple/iPhone данные и пользовательская браузерная сессия этим этапом не проверялись. Развёртывание относится к локальному Compose, не к удалённому production.

Этот отчёт и уточнение текущего состояния документации сохраняются отдельным коммитом после релизного тега; код выпуска не изменён.
