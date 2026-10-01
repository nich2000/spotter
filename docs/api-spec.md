# Спецификация API Spotter

Дата: 1 октября 2026 года. Версия: 1. Статус: целевой контракт для разработки и согласования, только документация. Описанные маршруты `/api/v2` пока не реализованы.

## 1. Область и приоритет

API реализует [Inbox](inbox-spec.md), [общую карточку](task-card-spec.md), [рабочий день](workday-spec.md), [проекты](projects-spec.md), [статистику](statistics-spec.md) и [дневник](wellbeing-spec.md) над [новой моделью данных](data-model-spec.md). Имена, связи, атомарность и неизвестные значения определяются новой моделью; старые `status`, `MITDate`, `MainDate`, строковый Project и обязательные Minutes не являются целевым контрактом. Все экраны используют один taskId — ID TaskOccurrence.

Решения R1–R13 из [общего каркаса](project-spec.md) остаются `proposal`. Наличие endpoint или примера не утверждает продуктовое решение. Сервер публикует доступные операции и выбранные политики; зависимую операцию блокирует с `POLICY_UNRESOLVED`, а зависимую метрику возвращает unavailable либо явно provisional при запрошенном предварительном расчёте. Независимые чтения и сохранение исходных данных работают без выбора всех политик.

Система локальная и однопользовательская. Нет внешнего назначения исполнителей, изменения Apple-источников, отправки комментариев или дневника модели. `completionActor=other` — запись пользователя о результате.

## 2. Транспорт, типы и версия

База: `/api/v2`. JSON UTF-8, lowerCamelCase. Все ответы личных данных: `Cache-Control: no-store`. Чтения — GET без побочных эффектов. Изменения workspace — единый `POST /api/v2/workspace/commands`, включая быстрые действия из любого раздела. Отдельный endpoint refresh только запускает сбор источников. GET не создаёт дневник, задачу, план или повторение.

Общие типы соответствуют модели: ID — непрозрачная непустая строка; Instant — RFC3339, ответ UTC; LocalDate — `YYYY-MM-DD`; LocalTime — `HH:mm[:ss]`; zone — существующий IANA-пояс. revision и entityVersion — целые неотрицательные JSON numbers не выше 2^53−1. Сервер запрещает переполнение. Длительности факта — миллисекунды; оценки — минуты. `null` означает неизвестное/отсутствующую связь. `[]` означает известный пустой набор, но не доказанную полноту истории.

В patch отсутствующий ключ означает «не менять», null — очистить nullable-поле; null для обязательного поля недопустим. Неизвестные поля, повторяющиеся ключи JSON, несколько JSON-значений в теле, NaN/Infinity, неверные enum/типы отклоняются. Ни один endpoint не принимает клиентские createdAt/updatedAt, completionEventId, source-derived счётчики или накопленный таймер как доверенные значения.

Успешный GET:

```json
{"apiVersion":"2","workspaceRevision":42,"serverNow":"2026-10-01T09:00:00Z","sourceSnapshotId":"src-19","data":{},"warnings":[]}
```

`sourceSnapshotId` nullable: источник имеет собственную версию, его refresh не означает правку локальной карточки. Все вложенные данные одного ответа читаются на одной workspaceRevision и одном указанном снимке источников.

### 2.1. Capabilities и проектный профиль валидации

`GET /api/v2/capabilities` возвращает `apiVersion`, `schemaVersion`, `validationProfile`, `policyVersions`, `commands[]:{name,available,blockedBy[]}`, `features`, `timerProtocol`, `legacyPolicy` и `migrationStatus`. Для каждой R-политики: `{id,status:proposal|selected,version:null|string,value:null|object}`. Значение proposal не считается selected даже при наличии демонстрационной формулы.

Ниже конкретный **предлагаемый технический профиль** `draft-1`, требующий утверждения/публикации перед выпуском (R11/R13), а не скрытая замена продуктовых лимитов:

| Ограничение | draft-1 |
|---|---|
| JSON request | 1 MiB; превышение 413 |
| ID / operationId / clientRef | 128 / 128 / 64 Unicode code points |
| Название задачи/подпункта | 1–500 символов после trim; название задачи 500 согласовано в карточке |
| Project name | 1–200; trim при новом вводе, миграционные имена не объединяются |
| Description / заметка дня | 20 000 символов |
| Resume, priorityNote, причины, комментарий, итог плана | 5 000 символов каждый |
| Подпункты / новые комментарии в save | 200 / 50 |
| Элементы плана / planChanges в карточке | 1 000 / 50; технические пределы не заменяют R4 |
| estimateMinutes / allocatedMinutes | 1–525 600 либо null |
| remainingMinutes | 0–525 600 либо null; 0 не завершает задачу |
| TimeEntry.durationMs | 0–31 536 000 000; это верхний технический размер, не допустимость пересечения |
| Счётчики привычек, шаги | целые 0–1 000 000; oxygen_saturation 0–100 |
| Другие измерения | конечные числа; неотрицательные, кроме температуры; медицинские пределы не устанавливаются |
| Проекты | глубина до 32 включительно, корень = 1; соседние одинаковые имена — R13 |
| Страница | default 50, max 200; период отчёта до 366 местных дат |
| heartbeat / потеря подтверждения | 15 / 45 секунд; публикуются как timerProtocol, не доказывают присутствие человека |
| Снимки чтения / отчёта | срок токена 10 минут, expiresAt в ответе |

Числовые/текстовые пределы измеряются сервером, клиент использует тот же профиль. Unicode code points — не байты и не UTF-16 units. Профиль может быть заменён только новой версией с совместимым обновлением клиента. Пока профиль не утверждён, документ даёт проект для реализации, а не обещание готового API. Лимиты Ready15/WIP3/MIT3/weekly5 публикуются отдельно в `legacyPolicy`; удалять их под видом смены транспорта нельзя (R4).

### 2.2. Единый согласованный срез и пагинация

Первое чтение списка возвращает `readToken`, `expiresAt`, `page:{limit,nextCursor,total}`. readToken фиксирует revision, serverNow и sourceSnapshotId независимо от endpoint; cursor непрозрачен и дополнительно фиксирует endpoint, фильтры, сортировку и последнюю пару сортировки. Следующая страница: тот же путь/фильтры, `readToken` и `cursor`. Смена endpoint/фильтра с прежним cursor — 400 `CURSOR_QUERY_MISMATCH`; истёкший токен — 410 `READ_TOKEN_EXPIRED`, клиент перечитывает список целиком. Сервер сохраняет неизменяемый read-снимок на срок токена; нельзя выдавать новую страницу на другой revision под старым токеном.

`GET /workspace?include=...` создаёт общий токен; последующие чтения проектов/задач/плана могут передать `readToken` для того же среза. Чтение без токена всегда актуальное. После SSE или записи клиент обновляет связанные представления совместно; старый срез допустим только с видимой датой обновления.

Порядок задач: `(createdAt ASC,id ASC)`; записи с неизвестным createdAt располагаются первыми по ID с `createdAt=null`, время миграции не выдаётся за создание. План/подпункты: `(position,id)`; история: `(sequence,id)`; измерения: `(measuredAt DESC,id)`. `total` — число во всей отфильтрованной выборке, а не на странице. Проектные и Inbox-счётчики имеют собственную указанную область и не зависят от раскрытия дерева.

## 3. Чтения для всех блоков

В таблице пути относительно `/api/v2`, query optional, если не отмечено required. `Task`, `Project`, `DayPlan`, `Measurement`, события и прочие DTO содержат все публичные поля одноимённых сущностей модели; внутренние legacy payload доступны только в диагностике миграции, не дублируются в обычной карточке. FK возвращаются ID, не названием.

| GET | Параметры и data |
|---|---|
| `/workspace` | `date`, `include` из tasks,projects,workday,inbox,settings,sources (default settings,workday,sources). Возвращает соответствующие проекции, revision и readToken; большие наборы имеют page/cursor, не обрезаются молча |
| `/tasks` | `state` (повторяемый lifecycle enum), `workMode`, `reviewRequired`, `importance`, `urgency`, `context`, `projectId` (ID либо `none`), `includeDescendants=false`, `planId`, `availableOn` (Instant), `q` (подстрока title/description), `limit,cursor,readToken`. Нет фильтра state — все состояния, UI явно запрашивает открытые. data.items — TaskSummary |
| `/tasks/{taskId}` | TaskCard: task, definition summary, recurrence rule\|null, subtasks[], comments первая страница, history первая страница, sourceContext, memberships[], timeSummary, warnings, availableActions. Никакие данные не теряются из-за свёрнутого UI |
| `/tasks/{taskId}/events` | `afterSequence`, `limit,cursor,readToken`; неизменяемые TaskEvent, включая reopened и migration_baseline |
| `/tasks/{taskId}/comments` | `limit,cursor,readToken`; комментарии с ID/time, без модели обсуждения |
| `/tasks/{taskId}/time` | сессии, интервалы, ручные записи и effective corrections; `from,to` Instants, page; confirmed/uncertain/manual-separated суммы |
| `/inbox` | `section=unreviewed\|q1\|q2\|q3\|q4\|all`, `date` для отметок плана, `q,projectId,limit,cursor`. items tagged `{kind:task,task}` или `{kind:candidate,candidate}`, sourceStatuses, counts всех секций. Предложения только в unreviewed; недоступные по inboxAvailableAt и done не входят в активную выборку |
| `/sources` | SourceConnection без секретов, SourceSyncState, ограничения/coverage, время последнего успеха; errorCode и пустой успех различимы |
| `/source-candidates` | `state=pending\|dismissed\|accepted`, `connectionId`, page; ID, candidateVersion, proposed fields, sourceItemIds, sourceSnapshotId, availability, acceptedOccurrenceId, ограничения identityMethod |
| `/source-candidates/{candidateId}` | полный исходный контекст, версия и происхождение; не заменяет пользовательскую карточку принятой задачи |
| `/projects` | `parentId=none\|ID`, `q`, `includeAncestors`, page. Поиск возвращает полный path и chain предков; пустые/неактивные проекты не исключаются |
| `/projects/{projectId}` | project, path, ancestors, childProjects page, задачи первая страница, summary с direct/subtree, attention [{taskId,reason,boundary}], summaryScope. Для группы без проекта использовать `/projects/unassigned` |
| `/workday` | `date` required; WorkdayProjection модели: планы/черновики выбранной даты, activePlan\|null, snapshot отдельно, focus, timer, timeSummary, completionMetrics, schedule, attention всех открытых задач независимо от плана |
| `/plans` | `fromDate,toDate` (последняя исключена), `status`, page; все версии, без подмены draft активным планом |
| `/plans/{planId}` | plan, items с текущими TaskSummary, budget, main/frog, volume outcomes; closed содержит snapshotId, живые поля не выдаются за snapshot |
| `/plans/{planId}/snapshot` | immutable PlanSnapshot; 404 SNAPSHOT_NOT_FOUND для незакрытого плана |
| `/schedule` | `planId` required, `readToken`; ScheduleProjection, включая неизвестный бюджет и sourceSnapshotId |
| `/focus` | occurrenceId\|null, selectedAt\|null, entityVersion |
| `/timer` | TimerState, WorkSession\|null, openInterval\|null, reconciliation\|null, serverNow, timerProtocol |
| `/recurrences` и `/recurrences/{ruleId}` | список page / rule+definition template, открытый occurrence, nextSlot, evaluations и diagnostics. `enabled` фильтр списка |
| `/wellbeing` | `fromDate,toDate` exclusive, page; только сохранённые записи и coverage, не сгенерированные «пустые дни» |
| `/wellbeing/{date}` | record\|null, source zone, measurements page, selected sleep/steps, candidates, taskActivity, rings, formulaVersion/policyVersions, coverage. Непосещённый день — 200 record:null |
| `/measurements` | `date`, `kind`, `sourceMetadataId`, `unassigned`, page; imported original и corrections различимы |
| `/measurements/{id}` | measurement, sourceMetadata, correctionChain, effective, coverage; исправление не уничтожает оригинал |
| `/settings` | WorkspaceSettings, TargetConfig, DailyLoadOverride выбранной `date`, политики и validationProfileVersion |
| `/operations/{operationId}` | квитанция коммита или 404 OPERATION_NOT_FOUND; отсутствие квитанции в момент чтения не доказывает, что ещё выполняющийся запрос не закоммитится |
| `/migration` | schemaVersion, manifest ID, warnings, coverage/baseline, validation results; без содержимого резервной копии и секретов |

`TaskSummary` содержит taskId, title, projectId/path, lifecycleState/workMode, context, importance/urgency, reviewRequired/reason, complexity, multiDay, estimateMinutes/remainingMinutes, deadline/scheduled/checkAt, recurrence summary, subtasksDone/Total, memberships для запрошенной даты, source attribution и attention. Полная карточка дополнительно содержит description, resume, priorityNote, waitingOn/backgroundReason и остальные поля модели.

Предложения календарного планировщика — `proposedBlocks[{taskId,startAt,endAt,zone,kind:focus|break}]`; это не факт работы. `/plans/{planId}/focus.ics?readToken=...` отдаёт text/calendar из того же среза. Нет фокусных блоков — 422 `NO_EXPORTABLE_BLOCKS`; экспорт не создаёт встречи и не изменяет план.

## 4. Единая команда, идемпотентность и ошибки

```json
{
  "operationId":"op-20261001-0001",
  "expectedRevision":42,
  "command":"task.create",
  "payload":{"title":"Проверить макет","projectId":null}
}
```

Заголовки POST: `Content-Type: application/json`, `X-Spotter-Request: workspace`. operationId обязателен; новый ID назначает клиент до отправки и сохраняет с замороженным телом запроса. Сервер назначает entity IDs и время.

Общий ответ 200 (включая create, чтобы повтор возвращал тот же статус):

```json
{
  "apiVersion":"2",
  "operationId":"op-20261001-0001",
  "committedRevision":43,
  "committedAt":"2026-10-01T09:00:02Z",
  "result":{"taskId":"task-101","definitionId":"def-101","clientRefs":{}},
  "affectedDomains":["tasks","inbox","projects"],
  "affectedIds":{"tasks":["task-101"]},
  "warnings":[]
}
```

Сначала сервер проверяет Origin/header, формат и квитанцию operationId; затем expectedRevision и доменные ограничения. Идентичный повтор возвращает первоначальную квитанцию с прежними IDs/revision/time даже после других изменений. Канонический hash включает command, payload, expectedRevision; изменение тела с тем же ID — 409 `IDEMPOTENCY_KEY_REUSED`. Срок хранения квитанций — срок workspace. События, данные и квитанция коммитятся вместе; успешная изменяющая транзакция увеличивает revision ровно на 1. No-op возвращает `result.noChange=true`, квитанцию на текущей revision и не создаёт доменное событие или разрез времени.

При timeout/обрыве/неоднозначном 5xx клиент повторяет **те же байты и operationId**, с backoff 1/2/4/8 секунд до 30 секунд между попытками; 429/503 учитывают Retry-After. Можно читать `/operations/{id}`. После 409 VERSION_CONFLICT автоматическая смена expectedRevision запрещена: черновик сохраняется, клиент перечитывает и сравнивает, после явного объединения отправляет новый operationId. SSE не закрывает черновик. При отказе ни комментарии, ни время, ни части карточки не сохраняются.

Стандартная ошибка:

```json
{"apiVersion":"2","error":{"code":"VERSION_CONFLICT","message":"Данные изменились в другой вкладке","retryable":false,"currentRevision":44,"fieldErrors":[],"details":{"expectedRevision":42,"affectedIds":["task-101"]}}}
```

| HTTP | Коды / обработка |
|---|---|
| 400 | INVALID_JSON, UNKNOWN_FIELD, INVALID_QUERY, CURSOR_QUERY_MISMATCH; исправить запрос |
| 403 | ORIGIN_REJECTED, REQUEST_HEADER_REQUIRED; запись не выполняется |
| 404 | ENTITY_NOT_FOUND, SNAPSHOT_NOT_FOUND, OPERATION_NOT_FOUND |
| 405 | METHOD_NOT_ALLOWED, заголовок Allow |
| 409 | VERSION_CONFLICT, IDEMPOTENCY_KEY_REUSED, CANDIDATE_CHANGED, CANDIDATE_ALREADY_ACCEPTED, PLAN_NOT_EDITABLE, ACTIVE_PLAN_EXISTS, TIMER_ALREADY_RUNNING, RECONCILIATION_REQUIRED, RECURRENCE_OPEN_CONFLICT, POLICY_UNRESOLVED |
| 410 | READ_TOKEN_EXPIRED, REPORT_TOKEN_EXPIRED; перечитать весь срез |
| 413 / 415 | REQUEST_TOO_LARGE / UNSUPPORTED_MEDIA_TYPE |
| 422 | VALIDATION_FAILED, INVALID_TRANSITION, UNFINISHED_SUBTASKS_DECISION_REQUIRED, INVALID_REFERENCE, PROJECT_CYCLE, INVALID_LOCAL_TIME, AMBIGUOUS_LOCAL_TIME, TIME_OVERLAP, INVALID_CORRECTION, NO_EXPORTABLE_BLOCKS |
| 429 | RATE_LIMITED, Retry-After; первоначальная операция повторяется с тем же ID |
| 500 / 503 | STORAGE_FAILED, SERVICE_UNAVAILABLE, SCHEMA_UNSUPPORTED; не показывать успешное сохранение; при неопределённом исходе искать/повторять квитанцию |

`fieldErrors[]:{path,code,message,params}` использует JSON Pointer, например `/payload/transition/checkAt`. Конфликт содержит текущие IDs/versions, но не личные тексты в логах. `POLICY_UNRESOLVED.details.decisionIds` перечисляет R-решения. Ошибка источника внутри успешного workspace GET — per-source status, не глобальный ноль и не исчезновение задач.

## 5. Задачи, карточка и предложения

`CardPatch` допускает title, description, resume, projectId, context, importance, urgency, priorityNote, complexity, multiDay, estimateMinutes, remainingMinutes, deadline, scheduled, inboxAvailableAt. Состояние и разбор — отдельные явные структуры команды, чтобы произвольный PATCH не обходил побочные эффекты.

`Deadline/CheckAt`: null либо `{kind:"date",date,zone}` / `{kind:"instant",at,zone}`. `Scheduled`: null либо `{startAt,endAt:null|Instant,zone,localStart,localEnd:null|string,offsetChoice:null|"earlier"|"later"}`. localStart/localEnd — местные `YYYY-MM-DDTHH:mm:ss` без offset; сервер проверяет соответствие Instant и zone. Несуществующее местное время — 422, неоднозначное без выбора — 422 с двумя вариантами. Date-only срок истекает на начале следующих местных суток; checkAt только датой становится актуальным с начала указанной местной даты. Эти две границы не взаимозаменяемы. Выход scheduled за deadline сохраняется как warning по модели; если выбранная R11 требует блокировки, это отдельная публикуемая политика.

`Transition` — tagged union:

- `{to:"not_started"}`;
- `{to:"in_progress",workMode:"active"|"paused",resume?:string}`;
- `{to:"in_progress",workMode:"background",backgroundReason:string,checkAt:Boundary}`;
- `{to:"waiting",waitingOn:string,checkAt:Boundary}`;
- `{to:"done",completionActor:"self"|"other"|"unknown",unfinishedSubtasksDecision?:"keep_open"|"complete_all"}`.

Done→открыто требует `reopen` с ожидаемым completionEventId, не обычный transition. `ReopenInput` = `{completionEventId:ID|null,migrationBaselineId?:ID}`: null допускается только для мигрированной done без достоверного завершения и требует ID её migration_baseline; сервер сверяет baseline и текущее done. Такое открытие записывает reopened_from_baseline, не придумывает дату/исполнителя завершения. Для обычной задачи completionEventId обязателен и должен совпадать с текущим. Выход из ожидания/фона очищает текущий контроль, историю и resume сохраняет. Любой уход фокусной задачи из active атомарно закрывает интервал и снимает фокус; WorkSession становится paused при паузе/фоне/ожидании, interrupted при завершении задачи до цели, TimerState прекращает отсчёт. Полностью подтверждённая цель может дать completed, но завершение задачи само по себе не добавляет полный Pomodoro.

`SubtaskInput`: существующий `{id,title,done,position}` либо новый `{clientRef,title,done,position}`. `subtasks`, если передан, заменяет весь список: отсутствие старого ID означает удаление, события сохраняются. ID другой задачи недопустим. clientRef уникальны в операции, ответ отображает их в серверные IDs. `commentsToAdd[]:{clientRef,text}` только добавляет; отсутствие массива сохраняет старые комментарии.

`Triage`: `{action:"complete",importance:"yes"|"no",urgency:"yes"|"no",priorityNote?:string}` или `{action:"require",reason:"manual",clearPriority:false|true}`. Complete явно снимает reviewRequired; изменение одного текста — нет. Значение clearPriority default false.

`PlanChange`: `{planId,action:"upsert",allocatedMinutes:int|null,position:int}` либо `{planId,action:"remove"}`. Удаление main/frog-элемента требует companion поля `clearMain:true` / `clearFrog:true`. Для нового taskId карточки ссылки подставляет сервер. Состояние target plan проверяется в общей транзакции; closed/discarded менять нельзя.

| command | payload → result и гарантии |
|---|---|
| `task.create` | `{title,projectId?:ID\|null,patch?:CardPatch,subtasks?:[],commentsToAdd?:[],planChanges?:[],recurrenceChange?:RecurrenceChange}` → taskId,definitionId,clientRefs. Базовая задача not_started, unknown приоритеты, reviewRequired; patch не может конфликтовать с title/projectId верхнего уровня |
| `task.save` | `{taskId,patch?:CardPatch,subtasks?:[],commentsToAdd?:[],triage?:Triage,transition?:Transition,reopen?:ReopenInput,planChanges?:[],timeEntriesToAdd?:TimeEntryInput[],recurrenceChange?:RecurrenceChange}` → taskId,clientRefs,eventIds,timeEntryIds. Все части карточки одной транзакцией; transition done и reopen взаимоисключающие |
| `task.triage` | `{taskId,triage}` → taskId; общий обработчик Triage для menu/drag |
| `task.transition` | `{taskId,transition}` → taskId,eventIds; готово без старта/плана разрешено |
| `task.reopen` | `{taskId,completionEventId:ID\|null,migrationBaselineId?:ID,then?:{to:"in_progress",workMode:"active"\|"paused"}}` → taskId,eventIds. Baseline-вариант по ReopenInput; default not_started, без фокуса/таймера; then — явный возврат на доску «В процессе» той же транзакцией |
| `task.move` | `{taskId,projectId:ID\|null}` → taskId,previousProjectId,projectId. Перенос только occurrence, общий handler CardPatch.projectId; прежний интервал разрезается на серверном T, текущий фокус сохраняется |
| `source.accept` | `{candidateId,candidateVersion,sourceSnapshotId,card?:{patch,subtasks,commentsToAdd,triage,transition,planChanges,timeEntriesToAdd,recurrenceChange}}` → taskId,definitionId,candidateId,clientRefs. Принятие+приоритет/план/завершение атомарны |
| `source.dismiss` | `{candidateId,candidateVersion,sourceSnapshotId}` → candidateId,state=dismissed, без Task |
| `source.restore` | те же поля → candidateId,state=pending; accepted восстанавливать нельзя |

Из source.accept без title берётся proposedTitle в сверенной версии. Уже accepted candidate с другим operationId возвращает 409 и existingTaskId; клиент открывает ту же задачу, не создаёт вторую. Повтор прежнего operationId возвращает исходную квитанцию. Изменившийся sourceSnapshotId не должен отвергать неизменившийся кандидат без причины: сервер сверяет candidateVersion и identity фактически использованного кандидата; отсутствие доступного снимка/неподтверждённая identity — CANDIDATE_CHANGED с актуальными данными.

`multiDay` — явный bool (default false), не вычисляется по числу минут; при false и известной оценке UI может показывать короткую (≤25 минут) или длительную задачу. Черновик карточки живёт на клиенте: открытие/редактирование/отмена не отправляют доменные команды. После save текст не теряется при конфликте. Серверное восстановление draft плана — отдельная функция §6, не сохранение каждой буквы карточки. Recurrence template не правится скрыто через CardPatch; явный recurrenceChange (§8) сохраняется в той же транзакции карточки. Отдельные команды §8 нужны для действий вне редактора и вызывают те же обработчики.

Пример атомарного сохранения:

```json
{
 "operationId":"op-card-2","expectedRevision":43,"command":"task.save",
 "payload":{
  "taskId":"task-101","patch":{"description":"Проверить мобильную версию","remainingMinutes":0},
  "subtasks":[{"clientRef":"row1","title":"Открыть на телефоне","done":false,"position":0}],
  "commentsToAdd":[{"clientRef":"comment1","text":"Проверено вместе с коллегой"}],
  "transition":{"to":"done","completionActor":"self","unfinishedSubtasksDecision":"keep_open"},
  "planChanges":[]
 }
}
```

## 6. План, снимок, бюджет и настройки

`PlanItemInput`: `{taskId,allocatedMinutes:int|null,position:int,volumeOutcome?:unknown|fulfilled|unfulfilled|continuation,volumeNote?:string|null}`. Новые item IDs создаёт сервер; существующий taskId в том же плане сохраняет item ID. Дубли taskId/position отклоняются. mainOccurrenceId и frogOccurrenceId nullable и обязаны входить в items; frog дополнительно требует importance=yes.

| command | payload → result |
|---|---|
| `plan.draft.save` | `{planId?:ID,date,zone,basedOnPlanId?:ID\|null,items:PlanItemInput[],mainOccurrenceId:ID\|null,frogOccurrenceId:ID\|null}` → planId,dayVersion,status=draft. Обновлять можно только draft; дата/zone созданного draft неизменны |
| `plan.draft.discard` | `{planId}` → planId,status=discarded; не открывает предшествующий closed |
| `plan.activate` | `{planId}` → planId,status=active. Один active на дату; конфликт требует явного закрытия прежнего, не скрытого replace. Создание membership/main history происходит при активации, draft ранее в них не участвовал |
| `plan.update` | `{planId,items:PlanItemInput[],mainOccurrenceId:ID\|null,frogOccurrenceId:ID\|null}` → planId,itemIds. Только active; полный состав, порядок и выделение меняются вместе |
| `plan.close` | `{planId,summaryText,nextDraft:{date,zone,items:PlanItemInput[],mainOccurrenceId:ID\|null,frogOccurrenceId:ID\|null}}` → planId,snapshotId,closedAt,nextDraftId,nextDayVersion. Пустой следующий подбор задаётся items=[] |
| `settings.update` | `{patch:{zone?,workStart?,workEnd?,reservePercent?,focusBlockMinutes?,scheduleBreakMinutes?,plannerSleepTargetHours?,longBreakMinutes?}}` → settingsVersion; только опубликованные доступные поля и диапазоны; отдельный focusBlock не подменяет Pomodoro 25 минут |
| `dayLoad.set` | `{date,zone,mode:auto\|low\|normal}` → date,mode; не меняет субъективное самочувствие дневника |

Если UI сразу «сохраняет план», допустима `plan.draft.save` с `activate:true` как явно атомарный вариант save+activate; default false. Она применяет те же проверки, создаёт ровно одну версию и возвращает итоговый active. Создание плана из карточки выполняется предварительным сохранением draft или атомарной активацией; ссылаться на выдуманный planId нельзя.

Все проверки nextDraft выполняются до закрытия. На одном T plan.close закрывает активный интервал, делает неизменяемый snapshot **до** изменения рабочих состояний, закрывает план, переводит текущую WorkSession в paused и останавливает отсчёт TimerState, снимает фокус, переводит бывшую активную фокусную задачу в paused, помечает незавершённые элементы для повторного разбора и сохраняет nextDraft. Снимок содержит исходные main/frog, поле состояния, проектный путь, подпункты и ссылки на время. Waiting/background нефокусных задач продолжаются. Ошибка любой записи откатывает всё. Повтор операции не создаёт второй snapshot/draft.

При наличии uncertain-сессии закрытие требует предварительной сверки (409 RECONCILIATION_REQUIRED), чтобы не превращать хвост после сна в подтверждённое время. Планирование нескольких будущих дат/членств включается только выбранной R5. `volumeOutcome` сохраняется как факт ввода, но метрика «сорвано» не появляется до R6. Старые лимиты до R4 явно доступны в capabilities; отсутствие оценки — warning и unknownEstimateCount, не 0 минут. Бюджет вычисляется по allocatedMinutes, доступному календарю и настройкам на согласованном срезе.

Проект settings.validationProfile публикует предлагаемые диапазоны: reservePercent 20–30, focusBlockMinutes 1–480, scheduleBreakMinutes 0–120, plannerSleepTargetHours >0 и ≤24, longBreakMinutes 1–120. WorkStart/end — LocalTime, end позже start в пределах одного рабочего дня; ночной рабочий график — отдельное расширение. Значения миграции вне нового профиля сохраняются с предупреждением до явного редактирования.

## 7. Фокус, таймер и ручное время

| command | payload → result |
|---|---|
| `focus.select` | `{taskId}` → focusOccurrenceId,pausedTaskId\|null; task context computer/anywhere, task переводится in_progress+active, прежняя фокусная задача paused, её интервал закрыт; новый таймер не запускается |
| `focus.clear` | `{}` → focusOccurrenceId=null; активный интервал приостанавливается, прежняя фокусная задача paused |
| `timer.start` | `{taskId,planId:ID\|null,kind:pomodoro\|focus,targetDurationMs?:int}` → sessionId,timer. taskId должен совпадать с focus; planId active и содержит task либо null вне плана. Для pomodoro target=1 500 000, для focus положительный target обязателен, предел профиля 28 800 000 |
| `timer.pause` | `{sessionId}` → sessionId,state=paused,confirmedActiveMs; прогресс не сбрасывается, задача/фокус сохраняются |
| `timer.resume` | `{sessionId}` → sessionId,state=running; task совпадает с фокусом, нет второго running, uncertain сначала сверяется |
| `timer.finish` | `{sessionId}` → sessionId,state=completed\|interrupted,confirmedActiveMs,completedPomodoros. Полный Pomodoro только по подтверждённой цели, досрочный interrupted; задача не завершается |
| `timer.break.start` | `{afterSessionId,phase:short_break\|long_break}` → timer phase/deadline, без ActiveInterval. Long break требует выбранной R9 длительности; после 4 полных циклов UI предлагает long, но не запускает |
| `timer.break.end` | `{}` → timer; следующий work не начинается автоматически |
| `timer.heartbeat` | `{sessionId,observedTimerVersion}` → serverConfirmedAt,timerVersion,sessionState; клиентский clock не задаёт длительность |
| `timer.reconcile` | `{sessionId,uncertainIntervalId,confirmedUntil:Instant,decision:discard_tail\|confirm_until,reason?:string}` → sessionId,confirmedActiveMs,discardedMs,correctionIds. Подтверждённая граница между lastConfirmedAt и серверной uncertainEnd; после сверки state=paused, явное resume отдельно |
| `time.add` | `{taskId,entry:TimeEntryInput}` → timeEntryId,effectiveTimeSummary |

`TimerState` для транспорта дополнен `status=idle|running|paused|awaiting_next|needs_reconciliation`, timerVersion, phase, sessionId|null, deadlineAt|null, cycleIndex, lastConfirmedAt|null. При idle phase=null. Серверные scheduler-переходы тоже транзакционны и увеличивают revision. При достижении целевой продолжительности рабочая сессия завершает отсчёт без автоматического завершения Task; перерыв требует явного start. Клиент вычисляет оставшееся по serverNow/deadlineAt, не числу setInterval.

Heartbeat — команда с revision/operationId; подтверждение сервера не подтверждает работу человека, только границу непрерывного соединения по выбранному техническому протоколу. Несколько вкладок используют одну sessionId, гонка даёт VERSION_CONFLICT; вкладка перечитывает timer и делает новый heartbeat с новым ID, не переносит старый на новую сессию. Потеря подтверждения дольше tolerance, сон/перезапуск переводят хвост в uncertain; время после lastConfirmedAt не входит в confirmed. `reconciliation` возвращает intervalId,start/end,lastConfirmedAt и причину. Окончание deadline без подтверждений не доказывает полный Pomodoro. Явное confirm_until — подтверждение пользователя, discard_tail завершает на lastConfirmedAt. Сервер не принимает будущую границу или границу позже uncertainty end.

`TimeEntryInput`:

```json
{"durationMs":1800000,"workDate":"2026-10-01","zone":"Europe/Moscow","kind":"additional","intervalStart":null,"intervalEnd":null,"targetSessionId":null,"targetTimeEntryId":null,"projectId":null,"overlapStatus":"unresolved","reason":"Примерная работа вне компьютера"}
```

kind=correction требует ровно один targetSessionId/targetTimeEntryId и overlapStatus=replacement, исходная запись сохраняется. Длительность — эффективная замена, а не добавка (0 допустим для обнуления ошибочного факта). Additional без даты либо без подтверждения отсутствия пересечения остаётся отдельной приблизительной величиной; для суммирования нужны workDate/zone и confirmed_disjoint. При известных intervalStart/End сервер проверяет end>start, соответствие duration и пересечения с эффективными интервалами/записями; нельзя обойти фактическое пересечение флагом confirmed_disjoint. Исправление задачи другого ID, циклическая correction chain или несогласованная дата — INVALID_CORRECTION. Интервал через полночь делится в отчёте по факту, ручная запись без интервала относится к workDate с явной approximate. Исторические коррекции доступны только при выбранной R11.

## 8. Повторения и источники

`RecurrenceInput`: `{frequency:daily|weekly|monthly,anchorDate,localStartTime:null|LocalTime,zone,weekday:null|1..7,monthDay:null|1..31,scheduledDurationMinutes:null|int,inboxLeadMinutes:int,deadlineOffset:null|{kind:date,days:int}|{kind:instant,minutes:int},shortMonthPolicy:null|string,dstPolicy:null|object}`. weekday обязателен только weekly, monthDay только monthly; anchorDate согласован с ними. Опережение draft-профиля 0–525600 минут; duration 1–525600; offset допускает отрицательный с абсолютным пределом 525600 минут/366 дней. Enum выбранных shortMonth/DST политик публикуется capabilities после R10/R12; клиент не может придумать policy ID. Сохранить enabled=false с нерешённой политикой можно, включить зависимое правило — POLICY_UNRESOLVED.

| command | payload → result |
|---|---|
| `recurrence.create` | `{taskId,template:CardTemplate,rule:RecurrenceInput,enabled:boolean}` → definitionId,ruleId,currentTaskId; явное превращение definition в recurring, текущая задача остаётся тем же ID; привязка исходного anchor slot выполняется один раз |
| `recurrence.update` | `{ruleId,ruleVersion,patch:Partial<RecurrenceInput>,template?:CardTemplate}` → ruleId,ruleVersion,templateVersion. Только будущие выполнения |
| `recurrence.setEnabled` | `{ruleId,enabled}` → ruleId,enabled; выключение не удаляет текущую задачу |
| `recurrence.preview` | это GET `/recurrences/{id}/preview?count=5`, count 1–20: slots с появлением, сроком, DST diagnostic; не создаёт occurrence |

`RecurrenceChange` внутри task.create/task.save/source.accept — tagged union: `{action:"create",scope:"future",template:CardTemplate,rule:RecurrenceInput,enabled:boolean}`; `{action:"update",scope:"future",ruleId,ruleVersion,patch:Partial<RecurrenceInput>,template?:CardTemplate}`; `{action:"setEnabled",scope:"future",ruleId,enabled:boolean}`. ID текущей задачи подставляет сервер. Scope future явный: редактирование текущей карточки не распространяет её поля на шаблон без переданного template; правило не меняет прошлые слоты. У новой задачи create связывает текущее occurrence с исходным anchor slot. Валидируются и текущая карточка, и template/rule; при любой ошибке не сохраняются ни поля карточки, ни правило, ни комментарии. Результат save дополняется ruleId/ruleVersion/templateVersion. Отмена карточки отменяет её черновое recurrenceChange.

`CardTemplate` — title, description, resume, projectId, context, importance, urgency, priorityNote, complexity, multiDay, estimateMinutes, remainingMinutes, subtasks:[{title,position}]. В template нет taskId, completed, sessions, comments, focus, планов и абсолютного дедлайна первого выполнения. Будущее scheduled строится из rule, deadline — из deadlineOffset. Преобразование существующей задачи с несовместимыми назначением/слотом требует явного исправления, не переносит её дату молча. График проверяется сервером при запуске и по расписанию, не клиентским polling; создание слота атомарно и дедуплицируется `(ruleId,slotKey)`.

Пропущенные слоты не создают очередь: не более одного открытого выполнения, после его завершения ближайший будущий слот исходной сетки. GET evaluations показывает skipped/coalesced. Повторное открытие при другом open даёт `RECURRENCE_OPEN_CONFLICT` с requestedOccurrenceId/openOccurrenceId/ruleId. Команды молчаливой замены/слияния нет до утверждения соответствующей R10; UI сохраняет ввод и показывает обе задачи. Это осознанно заблокированный продуктовый выбор, не потеря истории.

`POST /api/v2/sources/refresh`: `{operationId,connectionIds?:ID[]}` и те же Origin/header. Возвращает 202 `{operationId,refreshId,state:"queued"}`; повтор с тем же телом возвращает тот же refreshId, новое тело с прежним ID — 409. `GET /sources/refreshes/{refreshId}` — queued/running/completed/failed и результаты по connectionId. Источники не меняют workspaceRevision при обновлении только внешнего snapshot; acceptance проверяет candidateVersion в общей транзакционной границе. Receipt refresh хранится отдельно от workspace-команд, потому что expectedRevision не нужен для запуска внешнего чтения. Сбой одного источника сохраняет локальные задачи и последний успешный snapshot с устаревшей отметкой.

## 9. Проекты

| command | payload → result |
|---|---|
| `project.create` | `{name,parentId:ID\|null,position?:int}` → projectId,path. Пустая ветка допустима |
| `project.rename` | `{projectId,name}` → projectId,path; ID не меняется, ProjectEvent сохраняет before/after |
| `project.move` | `{projectId,parentId:ID\|null,position?:int}` → projectId,path,affectedDescendantIds. Необязательный UI первой версии, capabilities может отключить; проверка циклов/глубины и история дерева обязательны при включении |

Перенос задачи использует task.move. Одинаковое назначение — no-op; project rename тем же именем также no-op. «Без проекта» не projectId и не создаваемая папка. Удаление проектов с содержимым, массовый перенос и архивирование здесь не предоставляются. Соседние одинаковые имена допускаются только по выбранной R13; при unresolved конфликтном имени — POLICY_UNRESOLVED, существующие миграционные ветки остаются читаемыми. Проектные summaries включают потомков один раз и подписывают direct/subtree; закрытые задачи не создают attention.

## 10. Дневник, подтверждения и измерения

`wellbeing.save` payload:

```json
{
 "date":"2026-10-01","zone":"Europe/Moscow",
 "note":"После прогулки стало легче",
 "wellbeing":{"mood":"normal","energy":"medium","stress":"unknown","confirm":true},
 "habits":{"cigarettes":0,"alcoholPortions":0,"confirm":true},
 "measurementsToAdd":[{"clientRef":"steps1","kind":"steps","value":7200,"unit":"count","aggregation":"interval_total","measuredAt":"2026-10-01T18:00:00Z","intervalStart":null,"intervalEnd":null,"confirm":true}],
 "selectMeasurements":{"steps":{"clientRef":"steps1"}}
}
```

Для новых дневных ручных итогов measuredAt назначает сервер, если опущен; это время ввода, дата учёта берётся из date. `note` отсутствует — не менять. Отсутствующая группа wellbeing/habits — не трогать значения и подтверждение. Переданная группа содержит все свои поля и обязательный confirm; confirm=true устанавливает отдельный серверный confirmedAt, false сохраняет значения неподтверждёнными и очищает подтверждение этой группы. unknown/null не превращается в значение из UI-дефолта. Единственная уникальная запись date сохраняет первоначальный zone; попытка сменить её зону — 422 FIELD_IMMUTABLE.

Результат: `{dayRecordId,date,measurementIds,clientRefs,wellbeingConfirmedAt,habitsConfirmedAt}`. Запись, ручные измерения и выбранные ссылки сохраняются атомарно. Числа easy/medium/hard не принимаются от клиента. Сон и шаги — Measurement, а не параллельные изменяемые поля дневника.

`MeasurementInput`: `{clientRef,kind,value,unit,measuredAt?,intervalStart:null|Instant,intervalEnd:null|Instant,aggregation,confirm:boolean,correctsMeasurementId?:ID|null,provenanceNote?:string}`. Для ручного ввода SourceMetadata.kind=manual и receivedAt назначает сервер. source/device/sample ID импорта не принимаются как ручные. enum kinds/units/aggregation и диапазоны — модель §6. Давление требует `{systolic,diastolic}`; pulse обязательно различает instant/resting/daily_average. Для среднего/импортного окна нужен интервал; ручной дневной итог допускает неизвестное окно с полнотой, отмеченной как user_reported. sleepEnd при наличии принадлежит измерению сна/его provenance; по одному measuredAt дата пробуждения не выдумывается.

`selectMeasurements:{sleep?:{id}|{clientRef}|null,steps?:{id}|{clientRef}|null}` выбирает одно эффективное измерение соответствующего kind. Несколько перекрывающихся импортов не суммируются. Неизвестная атрибуция measurement допускает dayRecordId=null до явного выбора; ручное назначение сохраняет provenance, не переписывает observed interval.

| command | payload → result |
|---|---|
| `wellbeing.save` | описан выше; основной общий save |
| `measurement.add` | `{date?:LocalDate,zone?:IANA,measurement:MeasurementInput}` → measurementId,dayRecordId\|null; date создаёт дневник без неявного подтверждения других групп |
| `measurement.correct` | `{measurementId,replacement:MeasurementInput,reason:string}` → originalId,measurementId. correctsMeasurementId выставляет сервер; исходник immutable |
| `measurement.correction.setActive` | `{correctionId,active:boolean,reason:string}` → correctionId,active,effectiveMeasurementId; событие отмены/восстановления, не физическое удаление |
| `measurement.select` | `{date,kind:sleep\|steps,measurementId:ID\|null}` → dayRecordId,selectedId; FK/kind проверяются |

Коррекции/их отмена включаются по R11 и обязаны пересчитать выбранную эффективную ссылку с сохранением истории. Выбор raw imported записи не означает подтверждение нормального самочувствия. Существующий sleep bridge подключается серверным адаптером к тому же атомарному ingestion handler; публичного незащищённого HTTP импорта этим документом не вводится. Его envelope: `{connectionId,sourceSnapshotId,measurements:[{externalSampleId|null,deviceId|null,kind,value,unit,measuredAt,intervalStart,intervalEnd,aggregation,quality,completeness,payloadHash}]}`. Unique source identity/fingerprint обеспечивает повторяемость; изменённый образец с тем же ID сохраняет новую версию/происхождение, не тихо перезаписывает ручную коррекцию. При aggregate export_window сохраняется реальное окно, не предполагаемые фазы сна.

## 11. Отчёты, кольца и покрытие

`GET /reports?fromDate=2026-10-01&toDate=2026-11-01&zone=Europe/Moscow&calendarBasis=source|report&asOf=...&allowProvisional=false` создаёт неизменяемый отчётный срез. toDate исключена; asOf default serverNow и не позже serverNow; effective end=min(periodEnd,asOf). calendarBasis=source сохраняет календарь датированных сущностей; report — явная перегруппировка событий, сопоставление дат планов требует R12. Политика смешанных сохранённых поясов не угадывается.

Ответ `data`:

- `reportToken,expiresAt,periodStart,periodEnd,asOf,zone,calendarBasis,workspaceRevision,formulaVersion,policyVersions,coverage,status,reasons`;
- `summary:{completed,completedOutsidePlan,confirmedFocusMs,completedPomodoros,manualConfirmedMs,manualUnresolvedMs,startedNotCompleted,mainResultSuccess}`;
- `days[]:{date,confirmedFocusMs,manualConfirmedMs,completed,coverage,status,reasons}`;
- `projects[]:{projectId,path,direct,subtree,added,completed,openAtBoundary,confirmedFocusMs,treePolicy}`;
- `planning:{mainResultDays,carryovers,volumeOutcomes}`, `observations[]:{code,text,evidenceIds,periodStart,periodEnd,asOf}`.

Каждая метрика — `{value:number|null,unit,status:complete|partial|unavailable|provisional,reasons[],evidenceIds[],numerator?:number|null,denominator?:number|null}`. Для success rate denominator=0 → value=null и reason=NO_MAIN_RESULTS, не 0%. Число зарегистрированных событий при полной истории может быть 0; отсутствие измерений времени имеет reason=NO_MEASUREMENTS и не утверждает «не работал».

`GET /reports/{reportToken}/days/{date}` и `/reports/{reportToken}/projects/{projectId}` (`unassigned` для null) возвращают подробные задачи, события/интервалы и время **из того же среза**, с пагинацией. Клик дня в месяце не пересчитывает независимый дневной отчёт. Истёкший token — 410; клиент обновляет весь отчёт. Значения не суммируют уже включённые parent/subtree; неизвестные baseline не превращаются в исторические нули.

Coverage: `{domains:[{domain,knownFrom:null|Instant,baselineAt:null|Instant,gaps:[{start,end,reason}],completeness:complete|partial|unknown,reason:null|string}]}`. Миграционный текущий статус не восстанавливает прошлый остаток; повторное открытие не стирает реальные интервалы; confirmed время обрезается полуоткрытым пересечением с периодом, uncertain показывается отдельно. Correction replacement учитывается один раз. `outsidePlan` берётся из события завершения и не пересчитывается последующим добавлением в план.

`GET /wellbeing/{date}` возвращает отдельные кольца `{habits,activity,sleep}`. Каждое: `{fill:null|0..1,color:neutral|green|yellow|orange|red,status,completeness,reasons[],evidenceIds[],formulaVersion,boundaries,inputs}`. Внутреннее кольцо отдельно описывает fillStatus и colorStatus: известный сон при неизвестном mood даёт известную дугу с нейтральным цветом. Внешняя геометрия fill=1 не означает известные привычки. Прогресс по известным шагам может быть доступен при неизвестной истории задач, с partial. Неподтверждённые 0/0 не зелёные. Верхнего общего балла нет.

До выбора R1/R2/R3/R6/R8/R12 соответствующие окончательные итоги unavailable с `POLICY_UNRESOLVED` и decisionIds. `allowProvisional=true` позволяет явно маркированный расчёт по именованной proposal-version, указанной в ответе; не «утверждает» её для будущих запросов. Пример:

```json
{"value":null,"unit":"tasks","status":"unavailable","reasons":[{"code":"POLICY_UNRESOLVED","decisionIds":["R1"]}],"evidenceIds":[]}
```

Пример provisional R1: completed 2-го → reopened 3-го → completed 5-го даёт на границу 6-го один результат 5-го; самостоятельный дневник 2-го может использовать иной подписанный asOf. Без выбранной/показанной политики нельзя публиковать это расхождение как ошибку данных либо скрыто приводить к одному числу. Наблюдения детерминированы и раскрывают evidence, не диагностируют здоровье и не отправляют дневник модели.

## 12. Безопасность локального транспорта, SSE и совместимость

Текущее основание — [server.go](../internal/server/server.go), [workspace.go](../internal/server/workspace.go), [broker.go](../internal/sse/broker.go). Сейчас зарегистрированы GET `/api/state`, GET/POST `/api/workspace`, GET `/api/focus.ics`, POST `/api/refresh`, `/events`. Workspace POST проверяет `X-Spotter-Request: workspace`, Origin.host==Host, ограничивает тело 64 KiB и возвращает text errors; общая version — optimistic lock. SSE `/events` публикует `event:update` с AppState, без durable cursor. Это описание существующей реализации, не гарантия новой модели.

Для всех новых изменяющих маршрутов сохраняются защитный заголовок и Origin. Если Origin есть, проверяются scheme+host+port относительно настроенного локального origin; `Origin:null`, чужой origin и malformed отвергаются. Без Origin разрешён локальный CLI с защитным заголовком. CORS не включается, cross-origin preflight не разрешает запись. GET SSE нельзя требовать custom header от native EventSource, поэтому он остаётся чтением same-origin; данные не включаются в URL. Защита заголовком не является аутентификацией для публикации в сети. Существующее middleware localOnly проверяет RemoteAddr/X-Forwarded-For, но само по себе не доказывает loopback bind: при реализации сохранить/проверить реальное локальное прослушивание, не заявлять удалённый доступ.

`GET /api/v2/events` — отдельный поток, чтобы старый update/AppState не оказался неожиданно workspace-командой:

```text
event: workspace.changed
id: evt-104
data: {"revision":44,"domains":["tasks","plans","timer"],"operationId":"op-card-2"}

event: sources.changed
id: evt-105
data: {"sourceSnapshotId":"src-20","connectionIds":["calendar-local"]}
```

Дополнительные типы: `reset` (полное перечитывание), `capabilities.changed` (policy/profile/schema), `refresh.completed`. В событиях нет заметок, комментариев и измерений. Публикация происходит только после коммита. Event ID — транспортный cursor, не TaskEvent.sequence и не обязательно revision; source refresh имеет свою последовательность. SSE доставка best effort: Last-Event-ID воспроизводится только из доступного буфера, иначе reset с текущими revision/sourceSnapshotId. При подключении сервер отправляет handshake/reset с актуальными версиями; после offline/сна клиент всегда перечитывает timer и сверяет revision. Потеря SSE не теряет транзакцию. Клиент игнорирует уже обработанную revision, не применяет события как источник истины. Keep-alive комментарий каждые 20 секунд не заменяет timer.heartbeat.

Миграция транспорта:

1. Резервная копия и MigrationManifest, миграция на копии, проверка ID/coverage/receipt-инвариантов, затем атомарное переключение новой схемы и поддерживающего бинарника.
2. Новая UI использует capabilities и v2. До перехода старый API работает только со старой схемой. После перехода старые POST `/api/workspace` и несовместимый refresh возвращают 409 `CLIENT_UPGRADE_REQUIRED`, не делают двойную запись и не пытаются выразить дерево через Project string.
3. GET `/api/workspace` после перехода либо явный CLIENT_UPGRADE_REQUIRED, либо отдельно версионируемый read-only adapter с warnings о потерях; молчаливый lossy view не допустим. `/api/state` остаётся снимком источников/модельных выжимок, не источником фактов новой истории. Старый `/events` сохраняет AppState shape до вывода старого UI из эксплуатации; v2 живёт отдельно.
4. `/api/focus.ics` может быть read-only adapter на выбранный active plan с явно заданной датой; основной путь v2 фиксирует planId/readToken. POST `/api/refresh` при сохранении совместимости получает тот же Origin/header и запускает безопасный refresh adapter, не пишет legacy workspace.
5. Неизвестная schemaVersion → SCHEMA_UNSUPPORTED, записи запрещены; повреждённый файл не заменяется пустыми данными. Откат только совместным восстановлением архива и соответствующего бинарника, а не запуском старого кода поверх новой схемы.

## 13. Приёмка контракта и выполненная проверка документа

Матрица покрытия: Inbox — §§3,5,8; карточка — §§4–6,8; рабочий день — §§6–8; проекты — §§3,5,9; дневник — §§10–11; статистика — §11; транспорт/миграция — §§2,4,12.

Для реализации обязательны контрактные сценарии:

1. Два save на одну revision: один commit, другой 409; повтор первого после потери ответа возвращает ту же квитанцию, один comment/TimeEntry/event.
2. Одновременное принятие/приоритизация/завершение candidate — один taskId; refresh не затирает CardPatch, разные calendar instances не склеиваются.
3. Card save с ошибочным planId/подпунктом полностью откатывается; отмена клиентского draft не меняет сессию.
4. Focus switch, переход waiting/background/done, project move и plan.close сохраняют интервалы без пересечений; два timer.start не создают два running.
5. Сон/разрыв подтверждений, deadline, reconcile, manual correction: uncertain исключён; один и тот же факт не попадает дважды.
6. Plan.close создаёт snapshot прежнего состояния и один draft; discard draft не открывает закрытый план; две версии даты не дают два дня успеха.
7. Recurrence после пропуска/перезапуска создаёт максимум одно open; DST/короткий месяц и reopen collision дают явную диагностику до выбора политики.
8. Record:null, подтверждённый 0, unknown и неподтверждённый UI-default различимы; повтор manual/import не дублирует; исправление сохраняет original.
9. Общий reportToken даёт совпадение summary/day/project evidence; миграционная неполнота и unresolved policy не превращаются в 0.
10. Чужой Origin/нет header/неверный JSON/oversize отвергаются; SSE reset восстанавливает срез; старый клиент не пишет несовместимую схему.

Документ проверен в двух проходах: соответствие сущностей/операций новой модели и спецификациям блоков; затем сквозные границы atomic save, повторов/конфликтов, unknown, времени, proposals и совместимости. Это проверка документации, не выполненные HTTP/Go/browser-тесты и не подтверждение реализации endpoints.
