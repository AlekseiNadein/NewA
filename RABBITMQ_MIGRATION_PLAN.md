# RabbitMQ Migration Plan (Estimate Calc Queue)

Дата: 2026-06-30  
Проект: NAV SaaS MVP  
Контур: `estimate_calc_jobs` -> `calc_worker`

## Статус (2026-06-30)

**Cutover выполнен:** `APP_QUEUE_MODE=rabbit` в `run.bat`. Фазы 1–4 реализованы (outbox, publisher, consumer, idempotency, healthz, DLQ purge/replay, prefetch).

**Мониторинг:** `GET /api/healthz`, админка `/admin` → «Очередь» (`GET /api/admin/queue-stats`), ops-скрипты в `scripts/`.

**Откат:** `scripts/rollback-queue-db.bat` или `APP_QUEUE_MODE=db`.

Подробный handoff: `HANDOFF.md` (разделы «RabbitMQ очередь», «Текущая сессия 2026-06-30 RabbitMQ»).

## 1) Цель и критерии успеха

Перевести очередь расчета смет с DB-polling транспорта на RabbitMQ без простоя, с сохранением текущей бизнес-логики и статусов в БД.

Критерии успеха:

- Новые задачи публикуются в RabbitMQ и обрабатываются consumer-ом.
- Повторная доставка не ломает данные (идемпотентность).
- Есть DLQ + ручной replay.
- Можно безопасно откатиться в режим DB queue через флаг.
- Нет потери задач на cutover.

## 2) Текущая модель (as-is)

- Транспорт: PostgreSQL таблица `estimate_calc_jobs`.
- Claim: `FOR UPDATE SKIP LOCKED`, lease + retry + backoff + dead.
- Источник состояния строки: `app_estimate_lines.calc_status/calc_error/calc_json`.
- Consumer: `backend/internal/calcworker/worker.go`.
- Producer: enqueue в `backend/internal/store/file_store.go`.

## 3) Целевая модель (to-be)

### 3.1 Компоненты

1. Producer API/store:
- При постановке задачи пишет запись в outbox (в той же транзакции, где обновляет смету/строки).

2. Outbox publisher (новый фоновый компонент):
- Пакетно читает outbox.
- Публикует в RabbitMQ c publisher confirms.
- Помечает событие как published.

3. RabbitMQ:
- Exchange: `estimate.calc` (type: `direct`).
- Queue main: `estimate.calc.main`.
- Queue retry: `estimate.calc.retry.5s`, `estimate.calc.retry.30s`, `estimate.calc.retry.120s`.
- Queue DLQ: `estimate.calc.dlq`.
- Routing keys:
  - `estimate.calc` -> main
  - `estimate.calc.retry.5s|30s|120s` -> retry queues
  - `estimate.calc.dead` -> dlq

4. Consumer (`calc_worker`):
- Читает из `estimate.calc.main`.
- Выполняет текущий `processJob`.
- Обновляет `app_estimate_lines` и (опционально) `estimate_calc_jobs` для обратной совместимости.
- `ack` только после успешного DB commit.

5. DB (source of truth для состояния строки):
- `calc_status`, `calc_error`, `calc_json`, `calculated_at`.
- Таблица `estimate_calc_jobs` может жить как fallback на период миграции.

### 3.2 Контракт сообщения

Формат: JSON.

```json
{
  "messageVersion": 1,
  "jobId": "calcjob_xxx",
  "companyId": "cmp_xxx",
  "estimateId": "est_xxx",
  "lineId": "itm_xxx",
  "revision": 12,
  "code": "Е0624-001-05",
  "fgisSetId": "alrosa-2026-q2",
  "district": "14.3",
  "attempt": 1,
  "maxAttempts": 5,
  "createdAt": "2026-06-30T09:00:00Z",
  "traceId": "..."
}
```

Обязательные поля: `jobId`, `estimateId`, `lineId`, `revision`, `code`.

## 4) Идемпотентность и семантика обработки

Ключ бизнес-идемпотентности: `(estimate_id, line_id, revision)`.

Consumer перед расчетом:

1. Читает текущую строку сметы.
2. Если revision уже не совпадает -> сообщение stale:
   - лог + `ack` (или `dead` по политике), без повторной обработки.
3. Если `calc_status=done` и `calculated_at` уже свежий для того же revision -> `ack`.

Запись результата:

- В транзакции обновить `app_estimate_lines`.
- После commit сделать `ack`.

При любой доставке дубля итог остается тем же, побочных эффектов нет.

## 5) Retry/DLQ policy

Категории ошибок:

1. Временные (`timeout`, `network`, `too many connections`):
- retry с задержкой (5s -> 30s -> 120s), затем DLQ.

2. Бизнес-ошибки (`record not found`, некорректный payload, stale revision):
- без retry (сразу `dead`/DLQ), чтобы не шуметь.

3. Неизвестные:
- ограниченный retry до `maxAttempts`, затем DLQ.

Рекомендуемый источник attempt:
- `x-attempt` header + дублирование в payload для диагностики.

## 6) Конфигурация окружения

Добавить ENV:

- `APP_QUEUE_MODE` = `db` | `dual` | `rabbit` (default: `db`)
- `APP_RABBITMQ_URL` = `amqp://user:pass@localhost:5672/`
- `APP_RABBITMQ_EXCHANGE` = `estimate.calc`
- `APP_RABBITMQ_QUEUE_MAIN` = `estimate.calc.main`
- `APP_RABBITMQ_QUEUE_DLQ` = `estimate.calc.dlq`
- `APP_RABBITMQ_PREFETCH` = `8`
- `APP_OUTBOX_PUBLISH_BATCH` = `100`
- `APP_OUTBOX_PUBLISH_INTERVAL` = `1s`

Для `run.bat`:

- Установить `APP_QUEUE_MODE` и `APP_RABBITMQ_URL` рядом с остальными `APP_*`.
- На первом этапе оставить `APP_QUEUE_MODE=db`.

Для `scripts/restart-calc-worker.bat`:

- Пробрасывать те же `APP_QUEUE_MODE`/Rabbit ENV в `run-calc-worker-exec.bat`.

## 7) Изменения по коду (план по файлам)

### 7.1 Новые пакеты

- `backend/internal/queue/rabbit/`
  - `conn.go` (подключение, reconnect)
  - `topology.go` (exchange/queue bindings)
  - `publisher.go`
  - `consumer.go`
  - `message.go` (DTO + validate)

- `backend/internal/outbox/`
  - `store.go` (insert/fetch/mark published)
  - `publisher.go` (loop, confirms)

### 7.2 Изменения существующих

- `backend/internal/store/file_store.go`
  - В местах enqueue добавить запись в outbox в той же tx.
  - В режиме `dual` оставить и DB enqueue, и outbox.
  - В режиме `rabbit` можно не писать `estimate_calc_jobs` (или писать только audit/fallback).

- `backend/internal/calcworker/worker.go`
  - Вынести `processJob` в интерфейс, пригодный для DB и Rabbit transport.
  - Добавить адаптер для Rabbit message -> job model.

- `backend/cmd/calc_worker/main.go`
  - Выбор транспорта по `APP_QUEUE_MODE`.
  - В режиме `rabbit` запуск Rabbit consumer.
  - В режиме `dual` (опционально) запуск только Rabbit consumer, а DB worker оставить как fallback процессом на infra уровне.

- `backend/cmd/server/main.go`
  - Запуск outbox publisher goroutine при `dual|rabbit`.

### 7.3 DB миграции

Новая таблица `outbox_events`:

```sql
CREATE TABLE IF NOT EXISTS outbox_events (
    id TEXT PRIMARY KEY,
    topic TEXT NOT NULL,
    routing_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    published_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_unpublished
    ON outbox_events(created_at)
    WHERE published_at IS NULL;
```

Опционально (для идемпотентности доставок):

```sql
CREATE TABLE IF NOT EXISTS calc_message_receipts (
    message_id TEXT PRIMARY KEY,
    estimate_id TEXT NOT NULL,
    line_id TEXT NOT NULL,
    revision BIGINT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## 8) Порядок rollout (без даунтайма)

### Phase A: Infra + schema

1. Поднять RabbitMQ (dev/stage/prod), создать exchange/queues/dlx.
2. Применить SQL миграции outbox.
3. Добавить ENV в сервисы, но оставить `APP_QUEUE_MODE=db`.

### Phase B: Dual-write

1. Включить запись в outbox + publisher.
2. Проверить:
   - outbox не накапливается бесконечно;
   - publish confirms OK;
   - ошибки публикации видны в логах/метриках.
3. Основная обработка все еще через DB worker.

### Phase C: Canary consumer

1. Запустить Rabbit consumer на небольшой доле (например, по company allowlist).
2. Сверить:
   - латентность,
   - долю ошибок,
   - число дублей,
   - итоговые данные в `app_estimate_lines`.

### Phase D: Cutover

1. Переключить `APP_QUEUE_MODE=rabbit`.
2. Оставить DB worker выключенным, но fallback-код готовым.
3. Мониторить DLQ/latency/dead.

### Phase E: Stabilize + cleanup

1. Документировать runbook replay DLQ.
2. Решить судьбу `estimate_calc_jobs`: оставить как audit/fallback или удалить позже миграцией.

## 9) Мониторинг и алерты

Минимум:

- Gauge: queue depth (`main`, `retry`, `dlq`).
- Histogram: end-to-end processing latency.
- Counter: success/fail/dead/retry.
- Gauge: outbox unpublished count.
- Gauge: consumer connected (0/1).

Алерты:

- DLQ > 0 дольше N минут.
- Oldest message age > порога.
- Outbox unpublished растет > порога.
- Consumer disconnected > 1-2 минут.

## 10) План отката

Быстрый откат:

1. `APP_QUEUE_MODE=db`.
2. Перезапуск `run.bat` (поднимется старый DB worker контур).
3. Rabbit consumer выключить.
4. Outbox можно временно оставить включенным (не влияет на DB обработку).

Данные не теряются, если:
- producer по-прежнему пишет DB job (в `dual` режиме до полного cutover),
- либо гарантированно пишет outbox в той же tx.

## 11) Definition of Done

- Реализованы `db`, `dual`, `rabbit` режимы.
- Есть миграция outbox и успешный publish c confirms.
- Rabbit consumer в проде обрабатывает 100% новых задач.
- DLQ и replay-процедура протестированы.
- Откат на `db` протестирован и документирован.

## 12) Очередь задач на реализацию (предлагаемый порядок)

1. SQL миграция `outbox_events`.
2. Rabbit topology + minimal publisher.
3. Outbox publisher loop.
4. Dual-write из store.
5. Rabbit consumer + ack-after-commit.
6. Retry/DLQ policy.
7. Метрики и health.
8. Canary + cutover.
