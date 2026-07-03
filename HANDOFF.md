# Handoff — NAV SaaS MVP

План: `06_10_структурированный_контекст_пробного_проекта.md`.

## Запуск

**Одна команда для всей системы:**

```bat
run.bat
```

Перед стартом API (`:8080`) автоматически перезапускаются auth (`:8081`) и calc worker (фон).

| Процесс | Как запускается | Порт / роль |
|---------|-----------------|-------------|
| **Nginx** | `run.bat` → `scripts/restart-nginx.bat` | `:8080` — единая точка входа (UI + маршрутизация auth/app) |
| **Auth service** | `run.bat` → `scripts/restart-auth-server.bat` | `:8081` — login, users, companies, licenses |
| **API + frontend** | `run.bat` (основной процесс в текущем окне) | `:8090` — бизнес-API, статика (доступ через nginx `:8080`) |
| **Сервис расчёта** | `run.bat` → `scripts/restart-calc-worker.bat` | фон — очередь `estimate_calc_jobs`, расчёт позиций ГСН |

Ручной запуск (отладка, отдельное окно с логом в консоли):

| Команда | Режим |
|---------|--------|
| `run-auth.bat` | auth в текущем окне |
| `run-calc-worker.bat` | calc worker в текущем окне |

Скрипты в `scripts/`:

| Файл | Назначение |
|------|------------|
| `restart-auth-server.bat` | stop → build → фоновый старт |
| `restart-calc-worker.bat` | stop → build → фоновый старт |
| `run-auth-server-exec.bat` | env + `nav-auth-server.exe` |
| `run-calc-worker-exec.bat` | env + `nav-calc-worker.exe` → лог `data/calc-worker.log` |
| `restart-nginx.bat` | stop → setup (если нужно) → фоновый старт nginx |
| `setup-nginx.bat` | скачивание portable nginx в `tools/nginx/` |
| `start-observability.bat` | Docker: Loki + Alloy + Prometheus + Grafana |
| `stop-observability.bat` | остановка observability-стека |

**Observability (отдельно от `run.bat`):** нужен **Docker Desktop** + `scripts\start-observability.bat`. Подробности — раздел [Observability](#observability-2026-07-01).

**Проверка после `run.bat`:** четыре компонента — `nginx.exe` на `:8080`, `nav-auth-server.exe` на `:8081`, `nav-server.exe` на `:8090`, `nav-calc-worker.exe`; в логе worker нет `GSN database is not configured`.

Логин (рабочая база): **Система** / **nadein.av@yandex.ru** / **admin123**  
Демо из README (**admin@example.com**) в `data/app.json` может отсутствовать; в форме подставляется из черновика `localStorage`.

| Env | Назначение |
|-----|------------|
| `APP_ADDR` | внутренний порт app-сервера (`:8090`); публичный вход — nginx `:8080` |
| `APP_DATA_PATH` | legacy JSON snapshot: **только** settings (если нет PG); сметы/стройки — в `APP_DATABASE_URL` |
| `APP_AUTH_DATABASE_URL` | **auth-контур**: `auth.companies`, `auth.users`, `auth.company_licenses` (по умолчанию = `APP_DATABASE_URL`) |
| `APP_JWT_SECRET` | общий секрет JWT для app и auth (обязательно одинаковый при раздельных процессах) |
| `APP_DATABASE_URL` | стройки, объекты, сметы, строки, очередь расчёта, `app_settings` |
| `APP_GSN_DATABASE_URL` | `gsn.*`, `fgis_cs.*` |
| `APP_LOG_LEVEL` | `info` / `debug` / `warn` / `error` (observability) |
| `APP_LOG_FORMAT` | `json` (prod) или `text` (локальная отладка) |
| `APP_SERVICE_NAME` | имя сервиса в логах: `nav-api`, `nav-auth`, `nav-calc-worker` |
| `APP_LOG_FILE` | файл лога: `data\nav-server.log`, `data\nav-auth.log`, `data\calc-worker.log` |
| `APP_METRICS_ADDR` | Prometheus scrape только localhost: `:9090` / `:9091` / `:9092` |
| `APP_GRAFANA_URL` | ссылка в админке «Мониторинг» (default `http://localhost:3000`) |
| `APP_PROMETHEUS_URL` | ссылка в админке (default `http://localhost:9093`) |
| `APP_AUTH_METRICS_URL` | URL scrape auth metrics (default `http://127.0.0.1:9091/metrics`) |
| `APP_WORKER_METRICS_URL` | URL scrape worker metrics (default `http://127.0.0.1:9092/metrics`) |

Импорт справочников (не при старте): `import_regions`, `import_resource_codifier`, `import_fgis_cs`.  
Принудительный импорт users/companies из JSON: `go run .\backend\cmd\migrate_auth` (нужен `APP_AUTH_DATABASE_URL`).

## Auth-контур (фаза 4, 2026-06-30)

- **nginx** на `:8080` — единая точка входа; auth-маршруты → `:8081`, остальное → app `:8090`.
- Go reverse proxy (`internal/api/proxy.go`, `APP_AUTH_SERVICE_URL`) **удалён**.
- Конфиг: `deploy/nginx.conf`; установка nginx: `scripts/setup-nginx.bat`.
- App-сервис отдаёт только бизнес-API и статику; auth API — только через auth-сервис.

## Auth-контур (фаза 3, 2026-06-30)

- **users/companies/licenses убраны из `app.json` и `FileStore`** — источник истины только `auth.*` PG.
- JWT включает `name`, `authorized`; app-сервер проверяет доступ по claims (`Claims.CanAccessApp()`), **без lookup user** на каждый запрос.
- App-сервер читает auth PG только для `LicenseAvailable` и имён компаний (admin locks).
- `APP_AUTH_DATABASE_URL` **обязателен** для app-сервера.
- `APP_JWT_SECRET` **одинаковый** у `nav-server` и `nav-auth-server` (в `run.bat` / `run-auth-server-exec.bat`: `dev-secret-change-me`).
- После обновления — **перелогиниться** (старые JWT без `authorized` → 403 «учётная запись не авторизована»).

### Связь auth и расчёта смет

Auth **не участвует** в calc worker: worker не использует JWT, только `APP_DATABASE_URL` + `APP_GSN_DATABASE_URL`.

После выноса auth перезапуск `run.bat` **раньше не поднимал** calc worker → очередь копилась, UI показывал «В очереди». Сейчас `run.bat` перезапускает worker автоматически (см. раздел «Запуск»).

## Auth-контур (фаза 2, 2026-06-30)

- `backend/cmd/auth_server` — отдельный процесс `:8081` (`run-auth.bat`).
- `backend/internal/authapi` — HTTP handlers auth API.
- На фазе 2 auth-маршруты проксировались Go reverse proxy с `:8080`; с фазы 4 — через nginx.

## Auth-контур (фаза 1)

- Пакет `backend/internal/authstore` — PG-хранилище users/companies/licenses (схема `auth.*`, файл `db/auth_schema.sql`).
- При первом старте с пустой auth БД данные **автоматически** импортируются из `APP_DATA_PATH`.
- API login/register/users/licenses читают **authstore**, не `app.json`.
- Следующий шаг: отдельный процесс `auth_server` + proxy (фаза 2).

## Архитектура расчёта смет (2026-06-27)

### Принципы

1. **Источник истины — текстовая строка** (`raw_text` в БД; в UI — textarea / формат «Исходные данные»).
2. **Табличный редактор** — проекция текста: до расчёта только **исходный шифр + объём**; после worker — полная строка.
3. **Очередь** — транспорт задач для worker-ов (не для UI).
4. **БД** — источник состояния строки и результата (`calc_json`, `calc_status`).
5. **UI** — long-poll batch `GET /api/estimates/{id}/calc-batch` (основной путь с 2026-07-02); legacy polling `calc-status` отключён на frontend.

### Поток данных

```text
Текстовый редактор → parse структуры (frontend) → PUT /api/estimates
  → normalizeEstimateItem: объём из raw_text (backend)
  → app_estimate_lines (raw_text, quantity, revision, calc_status=queued)
  → estimate_calc_jobs (PostgreSQL queue)

calc worker service (отдельный процесс)
  → ClaimEstimateCalcJob (FOR UPDATE SKIP LOCKED, lease/retry/dead)
  → gsn.GetRecordDetail(code, fgisSet, district)
  → CompleteEstimateCalcJob → calc_json, calc_status=done

Табличный редактор (сразу после parse)
  → только исходный шифр (sourceCode) + объём + badge статуса
  → без запросов к /api/gsn/record

Табличный редактор (после worker)
  → batch-listener calc-batch (long-poll) → apply calcJson по батчам
  → оригинальный шифр, наименование, ед. изм., ресурсы, стоимость
```

### Batch-протокол расчёта (2026-07-02 — 2026-07-03, актуально)

**Контекст:** большие сметы (7500+ поз.) — polling `calc-status` перегружал UI и давал рассинхрон счётчика и суммы. Расчёт в очереди **не менялся** (Rabbit/outbox/worker); изменился только **транспорт результатов в UI**.

#### Два размера батча (`app_settings`)

| Ключ / UI | Env / API | Назначение | Default |
|-----------|-----------|------------|---------|
| `calc_start_batch_size` | «Батч постановки в очередь» | Сколько строк ставить в очередь за одну транзакцию при `POST .../calc` / `enqueueEstimateCalcBatches` | 50 |
| `calc_client_batch_size` | «Батч ответа клиенту» | Сколько **терминальных** статусов отдавать за один ответ `calc-batch` | 50 |

Размеры **независимы**: сервер может ставить в очередь по 50, клиент получать по 50 готовых.

#### Поколение расчёта `calc_generation`

- Колонка `app_estimates.calc_generation` (BIGINT, default 0).
- Входит в `revision` строки (`estimateLineRevision`: estimate + line + code + qty + **fgisSetId** + **district** + **calc_generation**).
- **`POST /api/estimates/{id}/calc/cancel`** — `calc_generation++`, сброс всех GSN-calc строк (status, calc_json, цены), dead jobs, pending outbox; возвращает `{ generation }`.
- Клиент передаёт `generation` в каждый `calc-batch`; при рассинхроне — `{ reset: true, generation }`, cursor `applied=0`.

**Важно (фикс 2026-07-03):** в `CancelEstimateCalc` нельзя `UPDATE` внутри `rows.Next()` — pgx `conn busy`. Сначала собрать строки, закрыть `rows`, затем обновлять.

#### API batch (маршруты в `server.go` — **до** `/calc-status` и `/calc`)

| Метод | Путь | Назначение |
|-------|------|------------|
| `GET` | `/api/estimates/{id}/calc-batch?applied=&generation=&wait=1` | Long-poll (~25 с): следующий батч терминальных статусов |
| `POST` | `/api/estimates/{id}/calc/cancel` | Отмена in-flight расчёта, bump generation |
| `POST` | `/api/estimates/{id}/calc` | Запуск enqueue (как раньше) |
| `GET` | `/api/estimates/{id}/calc-status` | Legacy polling (на frontend **выключен**) |

Ответ `calc-batch`: `generation`, `applied`, `total`, `processed`, `errors`, `grandTotal`, `done`, `items[]`, опционально `reset`.

Логика готовности батча (`store/calc_batch.go`): отдать батч, когда накопилось ≥ `calc_client_batch_size` **новых** терминальных строк **или** смета полностью терминальна (хвост).

#### Frontend (`web/app.js`)

Флаги:

```javascript
const ENABLE_ESTIMATE_CALC_BATCH_LISTENER = true;
const ENABLE_ESTIMATE_CALC_STATUS_POLLING = false;
```

**Контроллер** `estimateCalcBatchControllers`: `{ session, cursor: { applied, generation }, stopped, applying, listenerPromise }`.

- **`session`** — инкремент при `abortEstimateCalcBatchSession`; старый listener игнорирует ответы после abort.
- **`applied`** — курсор «сколько терминальных статусов клиент уже принял» (источник для счётчика «Обработано позиций»).
- **`generation`** — синхрон с сервером после cancel / batch.reset.

**Прогресс UI:**

- Счётчик: `batch.applied` / `total` (`useProgressOnly` в `updateCalcProgressTarget`), **не** `batch.processed` и **не** локальный подсчёт `calcStatus=done`.
- Сметная стоимость: инкремент `accumulateCalcGrandTotalFromPairs` по строкам текущего батча; база — не-ГСН строки.
- Анимация: `displayed` догоняет `processed` через `startCalcProgressAnimation`.

**Применение батча:** `applyEstimateCalcBatchResponse` → `applyEstimateCalcStatusPairs` (для large — `skipRecords` до `batch.done`) → обновление только **видимых** строк таблицы (`refreshEstimateTableRowsByIds`); полный remount — только при `batch.done`.

**Цены с индексом:** ячейка «Стоимость ед.» — `innerHTML` (`formatEstimateUnitPrice` с `<br>`); между батчами — `applyEstimateCalcDisplayFromRecord`.

#### Переключение текст → таблица (без изменений очереди)

1. `applyEstimateTextToEstimate` + `prepareEstimateTableCalculationState` (сброс GSN-строк локально).
2. `persistOpenEstimate` → `POST .../calc` → `startEstimateCalcBatchListener`.
3. Batch-listener до `batch.done`.

#### Смена сметного района / набора ФГИС **во время расчёта** (табличный режим)

Единая точка: **`restartEstimateCalculationAfterContextChange(estimateId)`** — вызывается из `applyDistrictDialog` и `updateEstimateFgisSet`.

```text
1. abortEstimateCalcBatchSession     — stop listener, session++, await promise
2. resetGsnLinesForTableCalculation  — локально: queued, обнулить цены/calcJson
3. restartEstimateTableCalculation   — UI: 0/N, displayed=0, runningGrandTotal=base
4. syncEstimateCalcDisplayAfterReset — немедленный refresh таблицы/шапки
5. POST .../calc/cancel              — generation++, сброс строк в БД (retry ×3)
6. PUT .../estimates                 — новый district / fgisSetId
7. POST .../calc                   — enqueue (ждёт освобождения calcStartJob до 12 с)
8. runEstimateCalcBatchListener      — applied=0, новая session
```

Ожидаемое UX: сразу **0/N** и **сметная стоимость = 0** (или база); затем рост по мере батчей.

**Backend при cancel:** `unmarkEstimateCalcStartJob`; stale goroutine enqueue прерывается проверкой `calc_generation` в каждом батче enqueue.

#### Восстановление редактора после F5 (2026-07-03)

- `sessionStorage.nav_editor_estimates` может быть урезан (квота) → пустые `items`.
- `rehydrateEmptyOpenEstimates()` после `refreshConstructionData` в `bootstrapAppData`.
- `ensureOpenEstimateHydrated()` в `renderEditor` при пустых items.
- Источник строк: `state.estimates` (полный список из `GET /api/estimates`).

#### Закрытие сметы / текстовый режим

- `cancelAndStopEstimateCalc` → abort session + `POST .../calc/cancel`.

#### Ключевые файлы

| Область | Путь |
|---------|------|
| Batch store/API | `backend/internal/store/calc_batch.go`, тесты `calc_batch_test.go` |
| Cancel / generation | `calc_batch.go` → `CancelEstimateCalc` |
| Enqueue | `file_store.go` → `enqueueEstimateCalcBatches`, `StartEstimateCalc` |
| Routes | `backend/internal/api/server.go` |
| Frontend batch | `web/app.js` — `runEstimateCalcBatchListener`, `restartEstimateCalculationAfterContextChange`, `abortEstimateCalcBatchSession` |
| Настройки UI | `web/index.html` — поля батчей; `?v=20260703c` |

#### Ловушки batch-режима

- **Cancel `conn busy`** — если снова 500 на cancel, расчёт после смены ФГИС не стартует (сброс UI есть, роста нет). Проверять лог API.
- **Не использовать `batch.processed` для счётчика** — это все terminal на сервере, клиент мог принять меньше.
- **Не вызывать `refreshAllRenderedEstimateTableRows` на каждый батч** — моргание на 7500 строк; только visible IDs.
- **Два listener'а** — всегда `abort` + await перед новым `runEstimateCalcBatchListener`.
- **`markEstimateCalcStartJob`** — второй `POST /calc` без cancel/unmark молча не ставит очередь (теперь retry + unmark on cancel).

### Поток данных (legacy polling — справочно)

На frontend **отключено** (`ENABLE_ESTIMATE_CALC_STATUS_POLLING = false`). Раньше:

```text
Табличный редактор → polling GET .../calc-status → apply calcJson.record
```

### Очередь `estimate_calc_jobs` (режим `APP_QUEUE_MODE=db`)

Статусы: `queued` | `leased` | `done` | `failed` | `dead`

- Уникальность: `(estimate_id, line_id, revision)`
- Lease ~45 с; просроченный `leased` снова забирается
- Retry с backoff; после `max_attempts` → `dead`
- Задачи создаются для GSN-позиций (`source=gsn`, непустой `code`) при `upsertEstimateDB`

### RabbitMQ очередь (2026-06-30)

Режим: `APP_QUEUE_MODE=rabbit` (текущий прод-контур).

Идемпотентность (фаза 3):
- Таблица `calc_message_receipts` (`estimate_id`, `line_id`, `revision`).
- Consumer перед обработкой вызывает `ShouldSkipCalcDelivery` (receipt / stale revision / already done).
- Receipt пишется в той же транзакции, что и `calc_status=done`.

Поток:

```text
PUT /api/estimates
  → outbox_events (в той же транзакции)
  → outbox publisher → RabbitMQ exchange estimate.calc
  → queue estimate.calc.main
  → calc_worker (RunRabbit) → gsn.GetRecordDetail → app_estimate_lines.calc_*
```

Очереди: `estimate.calc.main`, `estimate.calc.retry.{5s,30s,120s}`, `estimate.calc.dlq`.

**Health / метрики:** `GET /api/healthz` — mode, publisher/consumer status, counters, outbox backlog, queue depths, alerts, `pipelineReady`, `releaseReady`, блок `dlq` (current, growth10m, growthAlert), `history` (снимки очереди).

**Админка:** раздел **«Очередь»** (`/admin` → «Очередь») — `GET /api/admin/queue-stats` (тот же payload, только для админов): DLQ/main/outbox, pipeline/release ready, publisher/consumer, алерты, таблица истории. Автообновление каждые 30 с. Кэш: `admin.js?v=20260630a`.

**История DLQ:** ключ `app_settings.queue_stats_history` — до 120 точек, не чаще 1 раза в минуту (при опросе healthz / admin queue-stats). Рост DLQ за 10 мин → алерт `dlqGrowthHigh` при дельте ≥ 50.

**Consumer heartbeat:** отдельный процесс `calc_worker` пишет `app_settings.rabbit_consumer_heartbeat`; healthz считает consumer connected, если возраст ≤ 45 с (in-process counters в API-процессе не используются).

**Операции:**

| Команда | Назначение |
|---------|------------|
| `scripts/rabbit-queue-status.bat` | depths + healthz snapshot |
| `scripts/replay-rabbit-dlq.bat [limit]` | replay из DLQ в main (attempt=1) |
| `scripts/purge-rabbit-dlq.bat` | очистка DLQ (poison messages, без replay) |
| `scripts/wait-release-ready.ps1` | ожидание `releaseReady=true` в healthz |
| `scripts/smoke-rabbit-calc.ps1` | smoke-тест API → Rabbit → calc-status |
| `scripts/load-rabbit-calc.ps1 [N]` | лёгкий нагрузочный прогон (N PUT) |
| `scripts/rollback-queue-db.bat` | откат на legacy DB queue |

**Откат:** `APP_QUEUE_MODE=db` + `run.bat` (legacy DB worker).

**Алерты в healthz:** `publisherDisconnected`, `consumerDisconnected`, `outboxBacklogHigh` (>100), `dlqNotEmpty`, `dlqGrowthHigh` (рост DLQ ≥50 за 10 мин).

**releaseReady:** `pipelineReady && mode=rabbit && !dlqNotEmpty` — готовность к релизу при пустой DLQ.

**Tuning (prod):**

| Env | Рекомендация | Назначение |
|-----|--------------|------------|
| `APP_QUEUE_MODE` | `rabbit` | основной транспорт |
| `APP_RABBITMQ_PREFETCH` | `4` (1–32) | параллелизм consumer |
| `APP_OUTBOX_PUBLISH_BATCH` | `100` | batch publisher |
| `APP_OUTBOX_PUBLISH_INTERVAL` | `1s` | частота publisher |

**Release checklist:**

1. `GET /api/healthz` → `pipelineReady=true`, `queue.mode=rabbit`.
2. `scripts/smoke-rabbit-calc.ps1` → транспорт OK.
3. `scripts/load-rabbit-calc.ps1 5` → publisher/consumer растут, `pipelineReady=true`.
4. `scripts/rabbit-queue-status.bat` → `main` не копится, `dlq` под контролем.
5. После релиза: мониторить `outboxPending`, `depths`, `consumer.duplicates`.

**Откат одной командой:** `scripts/rollback-queue-db.bat` (переключает `APP_QUEUE_MODE=db` и перезапускает стек).

**Legacy:** `estimate_calc_jobs` + DB Manager используются только при `APP_QUEUE_MODE=db` (fallback).

### Строки `app_estimate_lines` (расширение)

`raw_text`, `parsed_json`, `calc_json`, `calc_status`, `calc_error`, `revision`, `calculated_at`

### Сервис расчёта

| Путь | Роль |
|------|------|
| `backend/internal/calcworker/worker.go` | Worker + Manager (динамическое число goroutine) |
| `backend/cmd/calc_worker/main.go` | Точка входа отдельного сервиса |
| `scripts/restart-calc-worker.bat` | Перезапуск (вызывается из `run.bat`) |
| `run-calc-worker.bat` | Ручной запуск в foreground |
| `data/calc-worker.log` | Лог фонового worker |

**Основной backend (`backend/cmd/server`) worker не запускает.**

Manager перечитывает `calcWorkerCount` из настроек каждые **5 с** и перестраивает пул goroutine (**1–8**, default **2**).

#### Защита от зависания (2026-06-30)

Симптом: процесс `nav-calc-worker.exe` есть, но `done` в `estimate_calc_jobs` не растёт, сотни задач в `leased` с истёкшим `leased_until`.

Причины:

1. **`calc_worker_count` слишком велик** (в БД было 16) при тяжёлых позициях ГСН (до ~100 ресурсов, N+1 запросов на ресурс).
2. **Пул GSN** был 8 соединений при 16 goroutine — половина ждала conn, lease истекал, задачи перехватывались повторно → `stale line revision`.
3. **Нет таймаута** на `GetRecordDetail` — goroutine не возвращалась в claim loop.

Исправления в коде:

| Механизм | Где |
|----------|-----|
| Таймаут расчёта позиции **40 с** (`jobProcessTimeout` < lease 45 с) | `calcworker/worker.go` |
| `gsn.Service.SetMaxOpenConns(workers + 4)` при старте Manager | `calcworker/worker.go` |
| `ReleaseStuckCalcJobLeases()` — `leased` с просроченным lease → `queued` | `store/file_store.go`, вызов при старте Manager |
| Максимум worker-ов **8** (было 16) | `store.NormalizeCalcWorkerCount` |

Диагностика:

```sql
SELECT status, count(*), max(updated_at) FROM estimate_calc_jobs GROUP BY status;
SELECT count(*) FROM estimate_calc_jobs WHERE status='leased' AND leased_until < now();
```

Ожидаемые ошибки в логе (не баг worker): `record not found`, `stale line revision` (устаревшие задачи в очереди).

### Настройки приложения

| API | Доступ |
|-----|--------|
| `GET /api/settings` | авторизованные пользователи |
| `PUT /api/settings` `{ calcWorkerCount }` | администраторы (`CanManageUsers`) |

Хранение: `app_settings` (PG) или `settings` в `data/app.json` (file mode).

**UI:** раздел «Настройки» → «Worker-ов расчёта» (`web/index.html`, `web/app.js`).

## API (основное)

Auth · CRUD строек/объектов/смет · `GET /api/estimates/{id}/calc-batch` · `POST .../calc/cancel` · `POST .../calc` · legacy `GET .../calc-status` · `GET/PUT /api/settings`

GSN: `supplements`, `hierarchy`, `regions`, `record?code&fgisSet&district`, `hierarchy-records`, `fgis-sets`, `fgis-rows`

## Редактор сметы

**Режимы:** текстовый (default) · табличный.

**Переключение текст → таблица:** `applyEstimateTextToEstimate` (parse) → `prepareEstimateTableCalculationState` → `restartEstimateTableCalculation` → `renderEditor` → `persistOpenEstimate` → `POST .../calc` → `startEstimateCalcBatchListener`. **Прямых запросов к ГСН нет** (`recalculateEstimatePricing` не вызывается).

**Смена района / набора ФГИС в табличном режиме (во время расчёта):** `restartEstimateCalculationAfterContextChange` — abort batch session → локальный сброс GSN-строк → UI 0/N → `POST .../calc/cancel` → persist → `POST .../calc` → новый batch-listener. См. раздел [Batch-протокол](#batch-протокол-расчёта-2026-07-02--2026-07-03-актуально).

**Шапка:** шифр, наименование, сметный район (`district`, `14.3`), набор ФГИС (`fgisSetId`), сметная стоимость.

### GSN-позиция: два этапа отображения

| Этап | Шифр в таблице | Прочие колонки |
|------|----------------|----------------|
| **До расчёта** (`estimateLineShowsGsnPartial`) | **Исходный** — `sourceCode` из текста, напр. `Е0624-001-05 (РМ59092РМ60193)` | Только **объём** + badge («В очереди» / «Считается» / …) |
| **После расчёта** (`calc_status=done`) | **Оригинальный** — `originalCode` из `calc_json` / БД | Наименование, ед. изм., стоимость; для работ — «+» и ресурсы |

Ключевые функции в `web/app.js`:

- `estimateLineSourceCode` / `sourceCode` — исходный шифр из текста (не затирается при расчёте)
- `estimateLineDisplayCode` — до calc: `sourceCode`; после calc: `originalCode`
- `estimateLineShowsGsnPartial` — урезанный вид строки
- `clearGsnLineCalcEnrichment` / `resetGsnLinesForTableCalculation` — сброс обогащения при смене района / набора ФГИС
- `restartEstimateTableCalculation` — обнуление счётчиков прогресса (позиции, ошибки, сметная стоимость) и `markEstimateCalcAwaitingServer`
- `restartEstimateCalculationAfterContextChange` — полный цикл cancel + restart при смене района/ФГИС **во время** batch-расчёта
- `runEstimateCalcBatchListener` / `abortEstimateCalcBatchSession` — long-poll batch-listener и инвалидация сессии
- `startCalcProgressAnimation` / `estimateCalcProgressTargets` — анимация счётчиков; источник прогресса — `batch.applied` из `calc-batch`, не legacy polling
- `rehydrateEmptyOpenEstimates` / `ensureOpenEstimateHydrated` — восстановление строк после F5 при урезанном sessionStorage
- `enrichEditorEstimateItems` — восстановление из `calc_json` при открытии сметы

При каждом parse текста GSN-строки **сбрасываются** до шифра+объёма (обогащение не сохраняется из прошлого состояния).

**Строка работы** (`isWork`): шифр, «+» → ресурсы (только после расчёта). **Позиция-ресурс** (С/М/Т): стоим. ед. из ФГИС.

**Badge статуса расчёта:** В очереди · Считается · Рассчитано · Ошибка.

**Исключение:** `/api/gsn/record` вызывается только при **ручном** раскрытии «+» у уже рассчитанной позиции (если ресурсы не в памяти).

Сессия редактора: `sessionStorage`; enriched-данные (children, цены) — в памяти + `calc_json` после worker.

**Формат «Исходные данные»:** строка `F(49)'наборФГИС=…*` — только параметр `наборФГИС` (без дублирования в `описание`).

### Поля позиции в исходных данных

Строка позиции (после шапки Э/Ю/К): поля разделены **апострофом** `'` (экранирования нет — только разделители). Пустое поле: `''`.

| № | Поле | Пример |
|---|------|--------|
| 1 | Шифр (исходный) | `Е0624-001-05 (РМ59092РМ60193)` |
| 2 | **Объём** | `(61,475)[4]` или `0,16.(3,68)` |
| 3 | Стоимость (часто пусто) | `` |
| 4 | Наименование | `…` |
| 5 | Ед. изм. | `м3` |

**Шифр для ГСН** (`extractSourceDataPositionCipher`): обрезка 1-го поля по ближайшему из `(`, пробел, `#`. Полный шифр с модификаторами хранится в `sourceCode` / `rawText`.

**Объём (2-е поле)** — парсится **только на backend** при `normalizeEstimateItem` из `rawText`:

- Арифметическое выражение (часто в скобках).
- Опционально `[N]` в конце — округление до N знаков после запятой.
- Операторы: `+` `-` `*` `/` и дополнительно **`.` умножение**, **`:` деление**.
- Дробная часть числа — **запятая** `,`; точка в выражении — **не** десятичный разделитель, а умножение.

Код backend:

- `backend/internal/store/quantity_expr.go` — `ParseSourceDataQuantity`, вычисление выражения.
- `backend/internal/store/source_data_fields.go` — `SplitSourceDataFields` (`strings.Split` по `'`).
- `normalizeEstimateItem` в `file_store.go` — подставляет `quantity` из 2-го поля `rawText`.

**Frontend** (`web/app.js`): при text→table разбирает только структуру (раздел/подраздел/позиция, шифр, `rawText`); **выражения объёма не вычисляет** (`quantity: 0` до ответа API). После `persistOpenEstimate` объёмы подставляются из ответа `PUT`. В текст обратно пишется исходное 2-е поле из `rawText` (`estimateSourceDataQuantityRaw`).

Фикстуры в корне:

- **Э10410.txt** — объёмы вида `(61,475)[4]`.
- **Э10420.txt** — объём с умножением через точку: `0,16.(3,68)` → `0,5888` (строка 49).

Тесты: `backend/internal/store/quantity_expr_test.go` (в т.ч. оба файла).

**Ловушка:** если на фронте снова появится `parseSourceDataQuantity`, ошибка вида `Строка N: неверное число "0,16."` — лексер принял `.` за часть числа вместо оператора умножения.

## Код

```
web/app.js, web/index.html, web/styles.css
web/admin.{html,js}
backend/internal/{api,store,gsn,presence,calcworker,outbox,queue}/
backend/internal/api/queue_health.go
backend/internal/store/{quantity_expr,source_data_fields}*.go
backend/cmd/{server,auth_server,calc_worker,migrate_auth,purge_dlq,replay_dlq}/
db/schema.sql, db/auth_schema.sql, db/gsn_schema.sql
run.bat, run-auth.bat, run-calc-worker.bat
scripts/restart-{auth-server,calc-worker,nginx}.bat
scripts/{purge,replay}-rabbit-dlq.bat, rabbit-queue-status.bat, smoke-rabbit-calc.ps1, load-rabbit-calc.ps1, rollback-queue-db.bat, wait-release-ready.ps1
RABBITMQ_MIGRATION_PLAN.md
```

## Админка (`/admin`)

Иконка: `Admin_Icon.ico` · токен: `nav_admin_token`

| Раздел | Содержание |
|--------|------------|
| **Пользователи** | CRUD |
| **Лицензии** | Квоты подразделов базы (суперадмин) |
| **Сметы** | Активные lock-сессии, принудительное завершение |
| **Очередь** | RabbitMQ: DLQ, рост за 10 мин, depths, pipeline/release, publisher/consumer, история (`/api/admin/queue-stats`) |

**Блокировки смет** (`presence.EstimateLocks`, TTL 90 с): `PUT/DELETE /api/estimates/{id}/lock`.

## Ловушки

- Оба PG URL обязательны; после `backend/**` — **`run.bat`** (перезапускает auth, calc worker и API).
- Без calc worker задачи копятся в `estimate_calc_jobs`, UI — «В очереди» / «Считается» без прогресса.
- **`calc_worker_count` > 4–8** на тяжёлых сметах может снова «заморозить» worker — смотреть `data/calc-worker.log` и SQL выше.
- После смены auth — **перелогиниться**; иначе 403 на `/api/estimates`, polling `calc-status` молча не обновляет UI.
- PUT сметы с пустым `items` не затирает строки.
- `go test ./backend/internal/store/ -run Quantity` — тесты объёма и фикстур Э10410/Э10420.
- Backup исходников перед правками: `backups/2026-06-27_*` (последние сессии).
- Не показывать `originalCode` до завершения calc — иначе в таблице виден шифр из ГСН вместо исходного из текста.
- `recalculateEstimatePricing` в `web/app.js` оставлена, но **не вызывается**; расчёт только через worker + polling.
- **Логин / sessionStorage:** битые `nav_editor_estimates` / `nav_editor_buffer` в `sessionStorage` роняли `app.js` до регистрации submit — кнопка «Войти» не работала. Чинится автоматически (`repairEditorSessionStorage` до `state`). Правило: `.cursor/rules/web-frontend.mdc`.
- **После логина** не вызывать `loadApp()` — только `bootstrapAppData()`; иначе гонка с начальным `void loadApp()`.
- **RabbitMQ:** без запущенного брокера (`APP_RABBITMQ_URL`) publisher disconnected, задачи копятся в `outbox_events`. DLQ растёт на poison messages (битые шифры ГСН) — `scripts/purge-rabbit-dlq.bat`, мониторинг в админке «Очередь».
- **Комбо «Сметные цены и индексы»:** не делать полный `mountEditorContent` при polling calc-status и после `loadFGISSets` — только обновление таблицы/опций селекта; remount откладывать при фокусе в шапке.
- **Grafana/Prometheus в админке:** ссылки работают только после `scripts\start-observability.bat` (Docker). Без Docker — раздел «Мониторинг» в админке всё равно показывает метрики; внешние UI недоступны.
- **Observability `/api/admin/monitoring`:** при падении scrape Prometheus-текста с auth/worker раньше был panic → nginx 502; исправлено (`expfmt.NewTextParser(model.UTF8Validation)`).

## Observability (2026-07-01)

Контекст: внедрение логирования, метрик и Docker-стека Grafana/Prometheus/Loki поверх существующего `healthz` и админки «Очередь». Приложение на Windows (`run.bat`), observability — в Docker на том же хосте.

### Архитектура

```
Приложение (Windows, run.bat)          Docker (observability)
─────────────────────────────          ───────────────────────
nginx :8080
nav-api :8090  ── metrics :9090 ──────► Prometheus :9093 ──► Grafana :3000
nav-auth :8081 ── metrics :9091 ──┘         ▲
calc_worker    ── metrics :9092 ──┘         │
data/*.log ──────────────────────────► Alloy ──► Loki :3100 ──┘
```

Три столпа:

| Столп | Реализовано | Где смотреть |
|-------|-------------|--------------|
| **Логи** | JSON slog, `request_id`, calc-поля (`estimate_id`, `line_id`, `code`) | `data/*.log`, Grafana → Loki (после Docker) |
| **Метрики** | Prometheus `/metrics` на каждом процессе | Grafana «NAV Overview», админка «Мониторинг», `:9093` |
| **Операционный health** | `healthz`, админка «Очередь» | `/api/healthz`, `/admin` → Очередь |

**Не реализовано (следующий этап):** OpenTelemetry + Jaeger, `traceparent` в Rabbit, Grafana alerting.

### Пакет `backend/internal/observability/`

| Файл | Назначение |
|------|------------|
| `init.go` | slog (level/format/file), HTTP listener `/metrics` |
| `middleware.go` | access log, `X-Request-ID`, HTTP Prometheus counters |
| `metrics.go` | registry, метрики очереди/calc/HTTP |
| `collector.go` | periodic gauge update: outbox pending, queue depths, consumer stats из БД |
| `snapshot.go` | сбор snapshot для админки; scrape `:9091`/`:9092` |
| `calc.go` | структурированные логи calc pipeline |
| `context.go` | обёртка над `requestctx` |
| `config.go` | `ConfigFromEnv` |

Связанные пакеты: `backend/internal/requestctx/` (request_id без цикла импортов store↔observability), `backend/internal/store/observability.go` (`QueueMetricsBridge`).

### Метрики Prometheus (`nav_*`)

| Метрика | Описание |
|---------|----------|
| `nav_http_requests_total{method,route,status}` | HTTP RPS |
| `nav_http_request_duration_seconds` | latency |
| `nav_estimate_calc_start_total` | `POST .../calc` |
| `nav_outbox_pending` | gauge |
| `nav_outbox_published_total` / `nav_outbox_failed_total` | outbox publisher |
| `nav_rabbit_connected{role}` | publisher / consumer |
| `nav_rabbit_queue_depth{queue}` | DLQ, main, retry |
| `nav_calc_processed_total{result}` | ok / retry / dead / duplicate / failed |
| `nav_calc_errors_total{stage}` | gsn / gsn_timeout / quantity / pricing / … |
| `nav_calc_duration_seconds` | полный calc |
| `nav_calc_gsn_duration_seconds` | GSN lookup |
| `nav_calc_pricing_duration_seconds` | `estimatecalc.LinePricingFromRecord` |
| `nav_calc_consumer_*` | persisted counters + heartbeat age (на API через collector) |

Scrape: `deploy/observability/prometheus.yml` → `host.docker.internal:9090|9091|9092`.

### Логи

| Процесс | Файл | Env |
|---------|------|-----|
| API | `data/nav-server.log` | `run.bat` |
| Auth | `data/nav-auth.log` | `run-auth-server-exec.bat` |
| Calc worker | `data/calc-worker.log` | `run-calc-worker-exec.bat` |

Формат JSON (`APP_LOG_FORMAT=json`). Correlation: `request_id` в HTTP → outbox payload (`requestId`) → Rabbit message → логи worker.

Пример поиска в Loki:
```logql
{job="nav"} | json | estimate_id="..."
{job="nav"} | json | request_id="..."
```

### Docker-стек (`deploy/observability/`)

| Сервис | Порт | Роль |
|--------|------|------|
| Grafana | `3000` | дашборды, логи (admin/admin) |
| Prometheus | `9093` | scrape метрик |
| Loki | `3100` | хранение логов |
| Alloy | — | tail `data/*.log` → Loki |

```bat
scripts\start-observability.bat   REM после перезагрузки Windows — снова вручную
scripts\stop-observability.bat
run.bat                           REM приложение (обязательно для данных в метриках)
```

Проверка: http://localhost:9093/targets — jobs `nav-api`, `nav-auth`, `nav-calc-worker` в состоянии **UP**.  
Дашборд: Grafana → папка **NAV** → **NAV Overview**.

### Админка `/admin`

| Раздел | API | Назначение |
|--------|-----|------------|
| **Очередь** | `GET /api/admin/queue-stats` | DLQ, outbox, pipeline/release ready, история, purge |
| **Мониторинг** | `GET /api/admin/monitoring` | сводка Prometheus + ссылки Grafana/Prometheus |

Мониторинг агрегирует: local metrics API + scrape auth/worker + данные очереди (consumer stats из `app_settings`, как в healthz). Автообновление 30 с.

Публичный health (без auth): `GET /api/healthz`.

### Запуск на Windows-сервере (чеклист)

1. PostgreSQL, RabbitMQ — как обычно
2. `run.bat` — приложение
3. Docker Desktop — запущен
4. `scripts\start-observability.bat` — Grafana/Prometheus/Loki
5. Админка → **Мониторинг** или Grafana `:3000`

### Известные проблемы / фиксы

| Симптом | Причина | Решение |
|---------|---------|---------|
| Grafana/Prometheus «нет ответа» | Docker-стек не запущен | `start-observability.bat` |
| Админка «Мониторинг» → 502 | panic при parse `/metrics` auth/worker | исправлено в `snapshot.go` (UTF8Validation) |
| Targets DOWN в Prometheus | `run.bat` не запущен или metrics порты закрыты | проверить `:9090-9092` на localhost |
| Auth/Worker metrics «нет» в карточках | процесс не слушает metrics | проверить `APP_METRICS_ADDR` в exec-скриптах |

### Следующие шаги observability

1. **OpenTelemetry + Jaeger** — distributed tracing (`OTEL_EXPORTER_OTLP_ENDPOINT`)
2. **`traceparent` в Rabbit** — сквозной trace calc pipeline
3. **Grafana alerting** — DLQ, outbox backlog, consumer down (правила поверх healthz-логики)
4. Опционально: probe доступности Grafana в админке (показывать «Docker не запущен»)

### Ключевые пути (git)

`backend/internal/observability/`, `backend/internal/api/{monitoring.go,queue_health.go}`, `deploy/observability/`, `scripts/start-observability.bat`, `web/admin.{html,js}` (раздел «Мониторинг»).

---

## Текущая сессия (2026-07-03, batch calc — стабильное состояние)

**Контекст:** batch-протокол доставки результатов расчёта в UI (см. [Batch-протокол](#batch-протокол-расчёта-2026-07-02--2026-07-03-актуально)). Очередь Rabbit/outbox/worker **без изменений**.

### Реализовано и проверено

| Область | Содержание |
|---------|------------|
| **Backend** | `calc_generation`, `calc_start_batch_size`, `calc_client_batch_size`; `GET calc-batch`, `POST calc/cancel`; cancel без `conn busy`; generation check в enqueue; `unmarkEstimateCalcStartJob` при cancel |
| **Frontend** | Batch-listener (`ENABLE_ESTIMATE_CALC_BATCH_LISTENER=true`); polling выключен; прогресс по `batch.applied`; инкремент grand total; refresh только visible rows; rehydrate после F5 |
| **Контекст mid-calc** | Смена района/ФГИС → reset UI → cancel → persist → calc → новый listener |
| **Тесты** | `backend/internal/store/calc_batch_test.go` |

### Стабильное поведение (acceptance)

1. **Text → table:** счётчик 0→N, сметная стоимость растёт по батчам; индексы в цене (`<br>`) видны до завершения расчёта.
2. **Большие сметы (~7500):** нет мгновенного 100%, нет моргания таблицы, сумма не «прыгает».
3. **F5 с открытой сметой:** строки восстанавливаются из `state.estimates`, расчёт продолжается.
4. **Смена ФГИС/района во время расчёта:** немедленно 0/N и базовая сумма → после cancel рост снова идёт.
5. **Закрытие сметы:** cancel + stop listener.

### Бэкапы сессии

`backups/2026-07-02_17-10_calc-batch/`, `2026-07-03_11-00_rehydrate-calc-reset/`, `2026-07-03_11-15_calc-session-reset/`, `2026-07-03_11-25_cancel-conn-busy/`

### Cache bust

`web/app.js?v=20260703c`

### Не делать без явной задачи

- Включать `ENABLE_ESTIMATE_CALC_STATUS_POLLING` параллельно с batch-listener.
- Использовать `batch.processed` или локальный `done`-count для UI-счётчика.
- `refreshAllRenderedEstimateTableRows` на каждый batch.
- `UPDATE` внутри `rows.Next()` в cancel.

### Следующие шаги (опционально)

1. Smoke/e2e: mid-calc FGIS change на смете 7500+ строк.
2. Метрики batch-listener (latency, batch size) в observability.
3. SSE вместо long-poll `calc-batch` (если понадобится снизить число HTTP-запросов).

---

## Текущая сессия (2026-06-30, RabbitMQ + админка DLQ)

Контекст: миграция очереди расчёта на RabbitMQ (фазы 1–4), мониторинг DLQ в админке. План: `RABBITMQ_MIGRATION_PLAN.md`.

### RabbitMQ: реализовано

| Фаза | Содержание |
|------|------------|
| **1** | `APP_QUEUE_MODE=rabbit` — без записи в `estimate_calc_jobs`; outbox в той же TX; permanent errors → DLQ без retry |
| **2** | Outbox publisher + Rabbit consumer; метрики; `GET /api/healthz`; ops-скрипты; consumer heartbeat в `app_settings` |
| **3** | `calc_message_receipts` — идемпотентность по `(estimate_id, line_id, revision)` |
| **4** | `APP_RABBITMQ_PREFETCH=4`; `pipelineReady` / `releaseReady`; purge/replay DLQ; нагрузочный smoke |

**Поток (prod):** `PUT /api/estimates` → `outbox_events` → publisher → `estimate.calc` → `estimate.calc.main` → `calc_worker` (RunRabbit) → GSN → `app_estimate_lines`.

**Ключевые пути:** `backend/internal/outbox/`, `backend/internal/calcworker/rabbit.go`, `backend/internal/queue/rabbit_inspect.go`, `backend/internal/api/queue_health.go`, `backend/cmd/purge_dlq`, `backend/cmd/replay_dlq`.

### Админка: мониторинг очереди

- Раздел **«Очередь»** в `/admin` — карточки статуса + таблица истории.
- API: `GET /api/admin/queue-stats` (admin only).
- История: `app_settings.queue_stats_history` (120 точек, интервал ≥1 мин).
- Алерт роста DLQ: `dlqGrowthHigh` при +50 за 10 мин.

### DLQ: purge vs replay

| Действие | Когда | Скрипт |
|----------|-------|--------|
| **Purge** | Poison messages (`record not found`, stale revision, хвост миграции) | `scripts/purge-rabbit-dlq.bat` |
| **Replay** | Временные сбои после исправления инфраструктуры | `scripts/replay-rabbit-dlq.bat [limit]` |

### Проверено

- `GET /api/healthz` → `pipelineReady=true`, `releaseReady=true`, `dlq.current=0` после purge.
- Админский endpoint отдаёт тот же payload с `history`.
- `run.bat`: `APP_QUEUE_MODE=rabbit`, RabbitMQ URL, prefetch, outbox batch/interval.

### Env (очередь, `run.bat`)

| Переменная | Значение |
|------------|----------|
| `APP_QUEUE_MODE` | `rabbit` |
| `APP_RABBITMQ_URL` | `amqp://guest:guest@127.0.0.1:5672/` |
| `APP_RABBITMQ_EXCHANGE` | `estimate.calc` |
| `APP_RABBITMQ_PREFETCH` | `4` |
| `APP_OUTBOX_PUBLISH_BATCH` | `100` |
| `APP_OUTBOX_PUBLISH_INTERVAL` | `1s` |

### Откат

`scripts/rollback-queue-db.bat` или `APP_QUEUE_MODE=db` + `run.bat` — legacy `estimate_calc_jobs` + DB worker.

### Следующие шаги (опционально)

1. Алерт/уведомление при `dlqGrowthHigh` (Grafana alerting или webhook).
2. ~~Grafana/Prometheus поверх healthz~~ — **сделано**, см. [Observability](#observability-2026-07-01).
3. OpenTelemetry + Jaeger (distributed tracing).

### Незакоммиченные изменения (сессия)

`run.bat`, `go.mod`, `go.sum`, `backend/internal/{api,store,calcworker,outbox,queue}/`, `backend/cmd/{server,calc_worker,purge_dlq,replay_dlq}/`, `web/admin.{html,js}`, `web/styles.css`, `scripts/*rabbit*`, `RABBITMQ_MIGRATION_PLAN.md`, `HANDOFF.md`

---

## Текущая сессия (2026-06-30, auth)

Контекст: вынос auth в отдельный сервис + PG ([транскрипт сессии](38c90377-ea92-47e6-8587-fe20b20621b9)) и последующая стабилизация calc worker.

### Auth: фазы 1–4 (реализовано)

| Фаза | Содержание |
|------|------------|
| **1** | `db/auth_schema.sql`, пакет `authstore`, автоимпорт из `app.json` при пустой auth БД |
| **2** | `backend/cmd/auth_server` (`:8081`), `authapi` |
| **3** | users/companies/licenses **только** в `auth.*`; `FileStore` без учёток; JWT с `authorized`; `withAuthorized` по claims |
| **4** | nginx `:8080` вместо Go-proxy; app `:8090` internal; `deploy/nginx.conf`, `scripts/setup-nginx.bat` |

Ключевые пути: `backend/internal/authstore/`, `backend/internal/authapi/`, `deploy/nginx.conf`, `backend/cmd/migrate_auth/`.

### Запуск: автоперезапуск всех составляющих

- `run.bat` вызывает `scripts/restart-auth-server.bat` и `scripts/restart-calc-worker.bat` **до** сборки и старта API.
- Правило агента: `.cursor/rules/backend-restart.mdc` — после правок backend проверять **три** процесса.
- Ручной foreground: `run-auth.bat`, `run-calc-worker.bat`.

### Calc worker: инцидент и фикс

**Инцидент:** после миграции auth расчёт смет «перестал работать». Процесс worker иногда был запущен, но зависал (0 `done`, сотни просроченных `leased`).

**Не путать с auth:** worker не зависит от JWT; проблема — отдельный процесс не перезапускался вместе с API + перегруз при 16 worker-ах.

**Фикс:** таймаут 40 с, масштабирование GSN pool, cap 8 worker-ов, сброс stuck leases при старте, лог `data/calc-worker.log`, автозапуск из `run.bat`.

### Проверено

- `run.bat` поднимает auth (`:8081`), API (`:8080`), calc worker.
- После фикса worker обрабатывает очередь (`done` растёт; ~тысячи позиций/мин на тестовой БД).
- Ошибки `record not found` / `stale line revision` — ожидаемы для битых шифров и устаревших revision в очереди.

### Следующие шаги

1. Оптимизация `listRecordResources` (убрать N+1) — если снова упираемся в таймаут 40 с.
2. Очистка/компактификация старых `estimate_calc_jobs` (`dead`, stale revision).
3. В UI «Настройки» — подсказка: рекомендуемо 2–4 worker-а.

### Незакоммиченные изменения (сессия)

`run.bat`, `run-auth.bat`, `run-calc-worker.bat`, `scripts/*`, `backend/internal/{calcworker,gsn,store}/`, `.cursor/rules/backend-restart.mdc`, `HANDOFF.md`

---

## Текущая сессия (2026-06-29)

### Сделано

1. **Табличный редактор — перезапуск расчёта при смене района / набора ФГИС:**
   - добавлена `restartEstimateTableCalculation(estimateId, estimate)` — сброс `estimateCalcProgressTargets` (`processed`, `errors`, `displayed` → 0), `markEstimateCalcAwaitingServer`;
   - вызывается в `applyDistrictDialog` и `updateEstimateFgisSet`, если открыт табличный режим;
   - тот же helper используется при text→table вместо дублирующего кода;
   - UX: сметная стоимость, «Обработано позиций» и счётчик ошибок снова идут от 0 и анимируются по polling (как при переключении на «Табличный»).
2. **Табличный редактор — комбо «Сметные цены и индексы»:** убраны лишние полные remount (`loadFGISSets`, polling calc-status); отложенный remount при фокусе в шапке.
3. **Логин после правок редактора:**
   - `repairEditorSessionStorage()` перенесён **до** инициализации `state`;
   - безопасный `hydrateOpenEstimatesFromSession` (массивы, `try/catch`);
   - после `POST /api/auth/login` — `renderShell()` + `bootstrapAppData()`, без повторного `/api/me`;
   - защита от гонки `loadAppRequestId`;
   - `showMessage` показывает ошибки (снимает `hidden`).
4. **Кэш:** `app.js` / `login-draft.js` → `?v=20260629a` в `index.html`.
5. **Правило Cursor:** `.cursor/rules/web-frontend.mdc` (`globs: web/**`).

### Проверено

- API: `POST /api/auth/login` + `GET /api/me` с cookie — 200 для `nadein.av@yandex.ru`.
- Старт `app.js` при `sessionStorage.setItem('nav_editor_estimates','null')` — `state` создаётся, скрипт не падает.

### Ловушки этой сессии

- В форме может подставляться **`admin@example.com`** из `localStorage` (`nav_login_draft`) — в `data/app.json` пользователя может не быть; это не баг UI.
- Не просить ручную очистку `sessionStorage` — авто-repair при каждой загрузке.
- Без `restartEstimateTableCalculation` при смене района/ФГИС старые значения `estimateCalcProgressTargets` не сбрасываются — счётчики «залипают», анимация не стартует с нуля.

### Следующие шаги

1. Ручная проверка: табличный редактор → смена сметного района и набора ФГИС — счётчики и стоимость с 0, анимация до итога.
2. При необходимости — подсказка в форме логина или сброс черновика с несуществующим e-mail.

### Незакоммиченные изменения

`web/app.js`, `web/index.html`, `web/styles.css`, `web/login-draft.js`, `.cursor/rules/web-frontend.mdc`, `HANDOFF.md`

---

## Текущая сессия (2026-06-27, продолжение)

### Сделано

1. **Исходный шифр** из 1-го поля: обрезка по `(`, пробел, `#` (frontend + backend).
2. **Парсинг объёма на backend:** выражения, `[N]`, операторы `*` `/` `.` `:`; запятая — десятичный разделитель.
3. **Разделители полей:** все `'` — разделители, без `''`-экранирования (`SplitSourceDataFields`).
4. **Frontend:** убран расчёт объёма; после сохранения — merge `quantity` из ответа API.
5. **Тесты** на `Э10410.txt`, `Э10420.txt` (в т.ч. `0,16.(3,68)`).

### Проверено

- **Э10410.txt** — `(61,475)[4]` и аналоги.
- **Э10420.txt** — строка 49: `0,16.(3,68)`; text→table без ошибки; объём после PUT.

### Следующие шаги

1. Перенести полный parse текста на backend (опционально).
2. SSE вместо polling calc-status (опционально).
3. Сохранять `children` ресурсов в БД или всегда восстанавливать из `calc_json`.
4. Удалить неиспользуемую `recalculateEstimatePricing` или оставить для не-GSN сценариев.

### Фикс контекста (2026-06-29)

**1. Набор ФГИС из строки `F(49)` при text→table**

- Симптом: в Текстовом редакторе строка `F(49)` содержит `наборФГИС=...`, но при переходе в Табличный в селекте показывалось «Не выбраны».
- Причина: в ряде сценариев приоритет отдавался устаревшему draft/состоянию, из-за чего `fgisSetId` не подхватывался из актуальной строки `F(49)`.
- Что закреплено в `web/app.js`:
  - при text→table разбор идёт по текущему `textarea`;
  - добавлен fallback-парсинг `наборФГИС`/`fgisSetId` из `F(...)` (regex), если разбор по split не сработал;
  - `fgisSetId` гидратируется из `sourceDataConfigLineRaw` при восстановлении/открытии сметы;
  - сериализация `F(49)` не теряет уже присутствующий `наборФГИС`.
- Проверка: смета с `F(49)'наборФГИС=alrosa-2026-q2*` открывается в Табличном с выбранным набором (не «Не выбраны»).

**2. Перезапуск прогресса расчёта при смене района / набора ФГИС**

- Симптом: при смене сметного района или комбо «Сметные цены и индексы» в табличном редакторе счётчики и сметная стоимость не обнулялись и не анимировались, в отличие от переключения text→table.
- Причина: `clearGsnLineCalcEnrichment` + `pollEstimateCalcStatus` без сброса `estimateCalcProgressTargets` и без `markEstimateCalcAwaitingServer`; `updateCalcProgressTarget` не откатывает `processed`/`displayed` назад.
- Что закреплено в `web/app.js`:
  - `restartEstimateTableCalculation` — единая точка сброса прогресса;
  - вызов из `setEstimateViewMode` (text→table), `applyDistrictDialog`, `updateEstimateFgisSet` (только если `estimateViewMode === "table"`).
- Проверка: в табличном режиме сменить район или набор → стоимость и «Обработано позиций: 0/N» с нуля, затем рост по polling.

### Незакоммиченные изменения

`HANDOFF.md`, `backend/internal/store/{quantity_expr,source_data_fields}*.go`, `file_store.go`, `web/app.js`, `Э10410.txt`, `Э10420.txt`, `tools/`

---

## Предыдущая сессия (2026-06-27)

### Сделано

1. **Очередь расчёта** в PostgreSQL + поля calc на строках сметы.
2. **Worker** вынесен из API в `calcworker` + `cmd/calc_worker`.
3. **API** `calc-status`, `settings`.
4. **Frontend:** polling статусов, badges, `rawText` при parse, persist при text→table.
5. **Настройки UI:** количество worker-ов (1–8).
6. **«+ Создать новую смету»** → `POST /api/estimates` + `refreshConstructionData()`; смета сразу в дереве «Стройки» (временно под объектом по умолчанию, пока не разобран текст).
7. **При сохранении сметы** (`persistOpenEstimate`) → `syncEstimateToConstructionTree`: по строке `Ю` создаёт/находит стройку и объект, переносит смету в нужный узел дерева.
8. **Табличный редактор до расчёта:** только исходный шифр (`sourceCode`) + объём; без клиентского fallback к ГСН.
9. **Табличный редактор после расчёта:** оригинальный шифр, наименование, ед. изм., ресурсы, стоимость — из `calc_json` через polling.
10. **Смена района / набора ФГИС:** `clearGsnLineCalcEnrichment` → persist → повторная очередь (без `recalculateEstimatePricing`).
11. **Формат «Исходные данные»:** `F(49)` — только `наборФГИС`, без дублирования в `описание` (смета Э7151769030).

### Проверено на примере

- Смета **Э7151769030** (формат «Исходные данные», `наборФГИС=alrosa-2026-q2`) — сценарий text→table→calc.
- Фикстура **Э10410.txt** в корне — типовой набор GSN-позиций с исходными шифрами вида `Е0624-001-05 (РМ…)`.
