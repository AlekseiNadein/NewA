# Контекст: подсистема «Состояние проектов» (ProjectStatus)

Дата фиксации контекста: 2026-07-17 (обновлено 2026-07-18).

**Реализация:** отдельный репозиторий `C:\Codex\ProjectStatus`  
**Краткий handoff в NAV:** раздел «Состояние проектов / ProjectStatus» в `HANDOFF.md`  
**Правило агента:** `.cursor/rules/project-status.mdc`

## Цель

Подсистема ProjectStatus должна:

1. получать доступный пользователю список строек;
2. после выбора стройки показывать аналитику по ресурсам всех входящих в неё смет;
3. читать предметные данные из PostgreSQL;
4. при необходимости запускать расчёт смет и контролировать его завершение через существующий NAV API.

## Главный архитектурный факт

`calc_worker` не является HTTP-сервисом. Это отдельный процесс, который читает задания из RabbitMQ (production-режим) или из PostgreSQL (legacy-режим), обращается к базе ГСН/ФГИС и сохраняет результат в app-БД.

Поэтому рекомендуемая граница интеграции:

- PostgreSQL app contour — read-only источник строек и готовой аналитики;
- NAV API — запуск расчёта и чтение его статуса;
- RabbitMQ и таблицы `outbox_events`/`estimate_calc_jobs` — внутренний транспорт, не публичный API нового сервиса;
- GSN-БД новому сервису не нужна для обычной аналитики готовых результатов.

## Текущая топология

```text
клиент (desktop / mobile)
        |
        v
ProjectStatus (:8100)  ← репозиторий C:\Codex\ProjectStatus
   | read-only SQL                         | HTTP + JWT (verify only)
   v                                      v
APP_DATABASE_URL                     nginx :8080 / NAV API + /login
   ^                                      |
   | результаты                           | outbox
calc_worker <--- RabbitMQ <--- outbox publisher
   |
   v
APP_GSN_DATABASE_URL (gsn.*, fgis_cs.*)
```

Процессы:

- nginx `:8080` — публичная точка входа;
- auth service `:8081`;
- NAV API `:8090`, обычно доступен через nginx;
- `nav-calc-worker.exe` — без HTTP-порта;
- **ProjectStatus** `:8100` — отдельно от `run.bat`;
- RabbitMQ — `estimate.calc.main`, retry-очереди и DLQ.

Локальные параметры запуска находятся в `run.bat` и `scripts/run-calc-worker-exec.bat`. Не переносить содержащиеся там dev credentials в новый репозиторий.

## Публичные точки входа нового сервиса

Через существующий nginx `:8080` публикуются два явных варианта интерфейса:

- `http://localhost:8080/projectStatusDesktop/` — desktop UI;
- `http://localhost:8080/projectStatusMobile/` — mobile UI.

URL без завершающего `/` должны отвечать постоянным redirect:

- `/projectStatusDesktop` → `/projectStatusDesktop/`;
- `/projectStatusMobile` → `/projectStatusMobile/`.

Явные адреса предпочтительнее автоматического определения устройства:
ссылки стабильны, оба варианта можно открыть на любом устройстве, а nginx
не зависит от ненадёжного анализа `User-Agent`.

### Внутреннее размещение

Базовый вариант — один процесс нового сервиса, например
`project-status-service :8100`, который отдаёт оба frontend bundle и BFF API:

```text
nginx :8080
  /projectStatusDesktop/  -> project-status-service:8100/projectStatusDesktop/
  /projectStatusMobile/   -> project-status-service:8100/projectStatusMobile/
  /api/project-status/graphql -> project-status-service:8100/api/graphql
```

Порт `:8100` — предлагаемый внутренний default; наружу он не публикуется.
Если desktop и mobile будут отдельными процессами, публичные URL сохраняются,
а меняются только nginx upstream.

### Реализованный nginx routing

Дополнение к `deploy/nginx.conf`:

```nginx
upstream project_status {
    server 127.0.0.1:8100;
}

server {
    # существующий listen 8080 и остальные location

    location = /projectStatusDesktop {
        return 308 /projectStatusDesktop/;
    }

    location ^~ /projectStatusDesktop/ {
        proxy_pass http://project_status;
        include proxy_params.conf;
    }

    location = /projectStatusMobile {
        return 308 /projectStatusMobile/;
    }

    location ^~ /projectStatusMobile/ {
        proxy_pass http://project_status;
        include proxy_params.conf;
    }

    location = /api/project-status/graphql {
        proxy_pass http://project_status/api/graphql;
        include proxy_params.conf;
    }
}
```

У `proxy_pass` для UI намеренно нет завершающего URI: upstream получает
исходный base path. Frontend обязан быть собран соответственно с base URL
`/projectStatusDesktop/` или `/projectStatusMobile/`; ссылки на JS/CSS,
manifest, service worker и client-side routes не должны начинаться от `/`.

GraphQL endpoint проксируется с внешнего
`/api/project-status/graphql` на внутренний `/api/graphql`.

### SPA fallback

Если варианты являются SPA, fallback должен выполняться внутри нового сервиса:

```text
GET /projectStatusDesktop/* -> desktop index.html
GET /projectStatusMobile/*  -> mobile index.html
```

При этом реальные отсутствующие assets (`.js`, `.css`, изображения) должны
возвращать `404`, а не `index.html`, иначе ошибки сборки маскируются HTML-ответом.

### Авторизация на новых URL

- единая страница входа: `http://localhost:8080/login?next=/projectStatusDesktop/`;
- `nav_session` уже имеет `Path=/`, поэтому cookie доступна обоим UI;
- запросы остаются same-origin относительно `localhost:8080`;
- desktop и mobile при `401` редиректят на `/login` с `next` на свой base path;
- BFF проверяет тот же NAV JWT и применяет тот же `companyId` tenant scope;
- токены не передаются через query string;
- nginx не принимает решений о правах — authorization остаётся в сервисе.

### API нового сервиса

Публичная точка:

```text
POST /api/project-status/graphql
```

Реализация находится в `C:\Codex\ProjectStatus\`. Оба UI используют одну
GraphQL-схему; различается только представление. В текущем MVP публичная схема
содержит дерево строек. Resource analytics и durable recalculation должны быть
добавлены в эту же схему, без отдельного desktop/mobile API.

## Модель данных

Иерархия:

```text
app_constructions
  1 -> N app_construction_objects
  1 -> N app_estimates
  1 -> N app_estimate_lines
```

Связи:

- `app_constructions.id`;
- `app_construction_objects.construction_id -> app_constructions.id`;
- `app_estimates.object_id -> app_construction_objects.id`;
- `app_estimate_lines.estimate_id -> app_estimates.id`;
- tenant — `company_id` на стройке, объекте и смете;
- PK строки сметы — `(estimate_id, id)`, `line_id` не глобален.

Ключевые файлы:

- `db/schema.sql`;
- `backend/internal/domain/models.go`;
- `backend/internal/store/file_store.go`.

### Результаты расчёта и ресурсы

В текущем рабочем дереве уже подготовлены специализированные таблицы:

- `estimate_calc_state` — текущее состояние расчёта сметы;
- `estimate_calc_lines` — рассчитанный snapshot позиции;
  поля включают `code`, `original_code`, `name`, `unit`, `determinant` (определитель
  из нормативной базы / `gsn.records.determinant`), `quantity`, `unit_price`,
  `total`, `resources_text`, `calc_status`;
- `estimate_calc_line_resources` — вклад ресурса в конкретную позицию;
- `estimate_calc_resources` — агрегат ресурса по смете.

Ключи:

- состояние: `estimate_calc_state.estimate_id`;
- строка: `(estimate_id, generation, line_id)`;
- ресурс строки: `(estimate_id, generation, line_id, resource_code, determinant)`;
- агрегат: `(estimate_id, generation, resource_code, determinant)`.

Поля агрегата ресурса:

- `total_consumption` — суммарный расход ресурса по смете;
- `estimate_price` — единичная сметная цена;
- `selling_price`, `transport_cost` — nullable, сейчас worker обычно их не заполняет;
- `name`, `unit`, `mass`, `cargo_class`, `corrections`;
- `generation` — поколение расчёта.

Ресурс идентифицируется парой `(resource_code, determinant)`, не только шифром.

Реализация:

- `backend/internal/estimatecalc/line_result.go` — `LineCalcSnapshot`, `ResourceContribution`;
- `backend/internal/store/estimate_calc_persist.go` — транзакционное сохранение и агрегирование;
- `backend/internal/store/file_store.go` — включение snapshot в `CompleteEstimateCalcJob`;
- `db/migrate_estimate_calc_tables.sql`.

Важно: эти файлы/изменения на момент фиксации контекста находятся в незакоммиченном рабочем дереве. Перед разработкой смежного сервиса нужно подтвердить, что миграция применена в целевой БД и этот контракт принят как стабильный.

Есть несогласованность bootstrap-схемы: runtime `ensureTreeSchema` добавляет
`app_estimates.calc_generation`, но в текущем `db/schema.sql` и
`db/migrate_estimate_calc_tables.sql` отдельного `ALTER TABLE ... ADD COLUMN`
нет. Запросы ниже предполагают, что приложение уже выполнило runtime-миграцию.
Перед использованием схемы в другом сервисе это нужно оформить явной SQL-миграцией.

## Как получить список строек

### Через существующий API

`GET /api/constructions`

Ответ — массив:

```json
[
  {
    "id": "con_...",
    "companyId": "cmp_...",
    "code": "4000",
    "name": "Название стройки",
    "createdAt": "...",
    "updatedAt": "..."
  }
]
```

Маршрут защищён JWT и автоматически ограничивает обычного пользователя его `companyId`. Superadmin видит все компании.

Для восстановления дерева также есть:

- `GET /api/objects`;
- `GET /api/estimates?summary=1`.

### Напрямую из БД

Новый сервис обязан сам применить tenant-фильтр:

```sql
SELECT id, company_id, code, name, created_at, updated_at
FROM app_constructions
WHERE company_id = $1
ORDER BY code, id;
```

Нельзя принимать `company_id` из произвольного query parameter без его проверки по доверенному JWT/service identity.

## SQL для аналитики выбранной стройки

### Состояние входящих смет

```sql
SELECT
    e.id AS estimate_id,
    e.code,
    e.title,
    e.district,
    e.fgis_set_id,
    e.calc_generation,
    cs.status AS calc_status,
    cs.lines_total,
    cs.lines_done,
    cs.lines_errors,
    cs.grand_total,
    cs.updated_at
FROM app_constructions c
JOIN app_construction_objects o ON o.construction_id = c.id
JOIN app_estimates e ON e.object_id = o.id
LEFT JOIN estimate_calc_state cs
       ON cs.estimate_id = e.id
      AND cs.generation = e.calc_generation
WHERE c.id = $1
  AND c.company_id = $2
ORDER BY o.code, e.code, e.id;
```

### Агрегат ресурсов по стройке

```sql
SELECT
    r.resource_code,
    r.determinant,
    r.name,
    r.unit,
    SUM(r.total_consumption) AS total_consumption,
    SUM(r.total_consumption * COALESCE(r.estimate_price, 0)) AS estimate_amount,
    MIN(r.estimate_price) AS min_estimate_price,
    MAX(r.estimate_price) AS max_estimate_price,
    COUNT(DISTINCT r.estimate_id) AS estimates_count
FROM app_constructions c
JOIN app_construction_objects o ON o.construction_id = c.id
JOIN app_estimates e ON e.object_id = o.id
JOIN estimate_calc_state cs
  ON cs.estimate_id = e.id
 AND cs.generation = e.calc_generation
 AND cs.status = 'done'
JOIN estimate_calc_resources r
  ON r.estimate_id = e.id
 AND r.generation = cs.generation
WHERE c.id = $1
  AND c.company_id = $2
GROUP BY r.resource_code, r.determinant, r.name, r.unit
ORDER BY estimate_amount DESC, r.resource_code, r.determinant;
```

Не усреднять цены молча. Разные сметы могут иметь разные `district`/`fgis_set_id`, поэтому одна пара `(resource_code, determinant)` может иметь разные цены. Для UX следует показывать диапазон цены либо добавлять группировку по расчётному контексту.

### Drill-down ресурса до сметных позиций

```sql
SELECT
    e.id AS estimate_id,
    e.code AS estimate_code,
    l.line_id,
    l.position_no,
    l.code AS position_code,
    l.name AS position_name,
    lr.consumption,
    lr.estimate_price,
    lr.unit
FROM app_constructions c
JOIN app_construction_objects o ON o.construction_id = c.id
JOIN app_estimates e ON e.object_id = o.id
JOIN estimate_calc_state cs
  ON cs.estimate_id = e.id
 AND cs.generation = e.calc_generation
JOIN estimate_calc_lines l
  ON l.estimate_id = e.id
 AND l.generation = cs.generation
JOIN estimate_calc_line_resources lr
  ON lr.estimate_id = l.estimate_id
 AND lr.generation = l.generation
 AND lr.line_id = l.line_id
WHERE c.id = $1
  AND c.company_id = $2
  AND lr.resource_code = $3
  AND lr.determinant = $4
ORDER BY e.code, l.position_no, l.line_id;
```

## Интеграция с расчётом

### Публично пригодный путь

1. Получить сметы выбранной стройки.
2. Для каждой сметы проверить состояние.
3. Если расчёта нет/он устарел — вызвать `POST /api/estimates/{id}/calc`.
4. Читать `GET /api/estimates/{id}/calc-status?summary=1`.
5. Считать готовой только смету с `status='done'` и
   `processed + errors >= total`: в persisted state `processed` отражает
   успешно рассчитанные строки, а ошибки идут отдельным счётчиком.
6. После готовности читать resource tables в одной read-only транзакции.

Контракт summary:

```json
{
  "total": 100,
  "processed": 100,
  "errors": 2,
  "grandTotal": 123456.78,
  "status": "done"
}
```

Для сметной стоимости authoritative поле — `grandTotal` из `calc-status?summary=1`/`estimate_calc_state.grand_total`. `app_estimates.total` — legacy и может быть `0` при корректно рассчитанных строках.

Дополнительные маршруты:

- `GET /api/estimates/{id}/calc-batch?applied=&generation=&wait=1` — UI-oriented long poll;
- `POST /api/estimates/{id}/calc/cancel`;
- `POST /api/estimates/{id}/calc?force=1` — принудительный полный rebuild;
- `GET /api/healthz` — состояние pipeline.

Для backend-to-backend сценария проще использовать `calc-status?summary=1`, а не воспроизводить cursor-протокол `calc-batch`.

## Архитектура команды пересчёта стройки

Принятые решения:

- пересчитывать только сметы, по которым отсутствует актуальный результат;
- использовать ту же пользовательскую авторизацию, что и в NAV;
- операция должна быть durable, идемпотентной и восстанавливаться после сбоев.

### Разделение ответственности

Новый analytics service:

- принимает команду пользователя на уровне стройки;
- проверяет JWT и tenant;
- создаёт внешний `operationId`;
- одним вызовом передаёт bulk-команду в NAV API;
- хранит/read-модель операции для UI и объединяет статусы смет.

NAV API:

- повторно проверяет JWT, роль и принадлежность всех смет компании;
- под блокировкой определяет, каким сметам действительно нужен расчёт;
- фиксирует идемпотентные команды и их `generation`;
- после commit самостоятельно и с retry ставит строки в outbox;
- является владельцем жизненного цикла расчёта отдельной сметы.

Calc worker по-прежнему ничего не знает о стройках и операциях: он получает
идемпотентные команды отдельных строк из RabbitMQ.

### Внешний endpoint analytics service

```http
POST /api/analytics/constructions/{constructionId}/recalculations
Authorization: Bearer <NAV JWT>
Idempotency-Key: <client-generated UUID>
Content-Type: application/json

{
  "mode": "missing-only"
}
```

Ответ:

```http
202 Accepted
Location: /api/analytics/recalculations/{operationId}
```

```json
{
  "operationId": "op_...",
  "constructionId": "con_...",
  "status": "accepted",
  "statusUrl": "/api/analytics/recalculations/op_..."
}
```

Повтор с тем же `Idempotency-Key`, tenant и стройкой возвращает ту же операцию.
Тот же ключ с другими параметрами должен возвращать `409 Conflict`.

### Bulk-контракт NAV API

Чтобы не хранить пользовательский JWT для фоновых retries, analytics service
должен передать команду в NAV во время исходного пользовательского запроса:

```http
POST /api/internal/recalculations
Authorization: Bearer <тот же пользовательский NAV JWT>
Idempotency-Key: <operationId>
X-Request-ID: <correlation ID>
Content-Type: application/json

{
  "constructionId": "con_...",
  "mode": "missing-only"
}
```

NAV сам получает список смет через
`construction -> objects -> estimates`; передавать доверенный список
`estimateIds` от клиента не требуется.

Ответ NAV:

```json
{
  "requestId": "recalc_...",
  "constructionId": "con_...",
  "status": "accepted",
  "items": [
    {
      "estimateId": "est_1",
      "status": "queued",
      "generation": 12
    },
    {
      "estimateId": "est_2",
      "status": "skipped",
      "reason": "result_available",
      "generation": 4
    }
  ]
}
```

Analytics service сохраняет только идентификаторы и поколения из ответа.
После `202` пользовательский JWT для выполнения расчёта больше не нужен.

### Почему не использовать существующий `/calc?force=1`

Он остаётся UI-контрактом, но недостаточен для durable orchestration:

- повтор не идемпотентен и может ещё раз увеличить `calc_generation`;
- ответ `202` не возвращает поколение;
- enqueue запускается фоновой goroutine процесса API;
- после сбоя между HTTP-ответом и enqueue нет durable-команды для восстановления;
- невозможно строго связать наблюдаемый `done` с конкретным запросом.

### Durable-команда в NAV

Предлагаемая таблица:

```sql
CREATE TABLE estimate_recalc_requests (
    id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL,
    company_id TEXT NOT NULL,
    construction_id TEXT NOT NULL,
    estimate_id TEXT NOT NULL REFERENCES app_estimates(id) ON DELETE CASCADE,
    generation BIGINT,
    requested_by TEXT NOT NULL,
    status TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, idempotency_key, estimate_id)
);
```

Статусы элемента:

```text
queued -> dispatching -> running -> done
                              \-> done_with_errors
          \-> retry_wait -> ...
          \-> failed
```

Статус общей операции вычисляется по элементам:

- `accepted` — команды зафиксированы;
- `running` — хотя бы одна команда выполняется;
- `done` — все элементы `done` или `skipped`;
- `done_with_errors` — есть terminal errors;
- `failed` — команда не смогла быть поставлена после лимита retries.

### Транзакция принятия команды

NAV для bulk-запроса:

1. проверяет JWT через текущие `withAuth` + `withAuthorized`;
2. проверяет `claims.Role.CanEditEstimates()`;
3. выбирает сметы только через `company_id` из claims;
4. блокирует сметы `FOR UPDATE` в стабильном порядке по `id`;
5. повторно проверяет наличие результата;
6. для `skipped` фиксирует причину без смены generation;
7. для остальных однократно увеличивает `calc_generation`, инвалидирует старые
   jobs/outbox/results и создаёт `estimate_recalc_requests`;
8. commit;
9. возвращает поколение каждой сметы.

Уникальность `(company_id, idempotency_key, estimate_id)` делает повтор
безопасным: возвращается ранее созданная команда и то же поколение.

### Надёжная постановка строк

Отдельный NAV dispatcher забирает `queued/retry_wait` команды через
`FOR UPDATE SKIP LOCKED`, формирует строки текущего поколения и пишет
`outbox_events`.

Для безопасного повтора dispatcher-а нужен уникальный ключ outbox:

```text
calc:<estimateId>:<generation>:<lineId>:<revision>
```

Текущий `outbox_events.id` создаётся случайно, поэтому одного idempotency
consumer-а недостаточно: две копии сообщения могут одновременно попасть в
worker до появления receipt. Следует добавить `dedupe_key TEXT UNIQUE` и
вставлять событие через `ON CONFLICT (dedupe_key) DO NOTHING`.

После фиксации всех outbox events dispatcher переводит команду в `running`.
Сбой процесса до этого момента безопасен: другой dispatcher повторит операцию.

### Определение «результат отсутствует»

Проверка должна опираться на состояние текущего поколения, а не на наличие
строк в `estimate_calc_resources`, поскольку корректный расчёт может дать
пустой список ресурсов.

Базовый predicate:

```sql
cs.estimate_id IS NULL
OR cs.generation <> e.calc_generation
OR cs.status <> 'done'
```

Принятая продуктовая политика: `status='done' AND lines_errors > 0`
считается имеющимся частичным результатом. Автоматическая команда
`missing-only` такую смету пропускает; её повторный расчёт возможен только
отдельной явной командой.

### Статус операции

```http
GET /api/analytics/recalculations/{operationId}
```

```json
{
  "operationId": "op_...",
  "status": "running",
  "total": 12,
  "skipped": 7,
  "queued": 1,
  "running": 2,
  "done": 2,
  "doneWithErrors": 0,
  "failed": 0,
  "items": [
    {
      "estimateId": "est_1",
      "generation": 12,
      "status": "running",
      "processed": 80,
      "errors": 1,
      "total": 100
    }
  ]
}
```

Готовность конкретной сметы проверяется только для generation, возвращённого
при принятии команды. Если текущий `app_estimates.calc_generation` стал больше,
операция помечается `superseded`, а результат нового чужого запуска не
приписывается старой операции.

### Авторизация

- браузер продолжает использовать `nav_session` или Bearer JWT;
- analytics service проверяет тот же JWT и проксирует его только при принятии
  bulk-команды;
- NAV повторно проверяет JWT и tenant; доверять переданному `companyId` нельзя;
- analytics service не получает права подписывать JWT и не хранит
  пользовательский токен для фоновой обработки;
- желательно разместить analytics endpoint за тем же nginx origin, чтобы
  существующая cookie работала без нового CORS/auth flow.

### Чего не делать

- не вставлять задания напрямую в `estimate_calc_jobs`;
- не писать напрямую в `outbox_events`;
- не публиковать сообщения в `estimate.calc` из нового сервиса;
- не читать незавершённое поколение как готовую аналитику;
- не агрегировать `calc_json` в runtime, если доступны нормализованные resource tables;
- не использовать `app_estimates.total`.

## Auth и tenant isolation

NAV API принимает:

- cookie `nav_session`; или
- `Authorization: Bearer <JWT>`.

JWT содержит `sub`, `companyId`, `role`, `authorized`, `exp`. Все бизнес-маршруты требуют валидный JWT и `authorized=true`.

Отдельного machine-to-machine/service-account контракта сейчас нет. До production-интеграции нужно выбрать один вариант:

1. предпочтительно — добавить service account/client credentials с ограниченными scopes;
2. временно — выделенный технический пользователь и Bearer JWT с безопасным обновлением;
3. проксировать запросы пользователя и сохранять его tenant context.

Нельзя вшивать общий `APP_JWT_SECRET` в браузер или выдавать новому frontend прямой доступ к PostgreSQL.

Рекомендуемые роли БД:

- analytics reader: `SELECT` только на `app_constructions`, `app_construction_objects`, `app_estimates`, `estimate_calc_state`, `estimate_calc_lines`, `estimate_calc_line_resources`, `estimate_calc_resources`;
- migrations owner — отдельно;
- никаких `INSERT/UPDATE/DELETE` для runtime analytics service.

## Согласованность

- Расчёт версионируется `calc_generation`.
- Аналитика должна соединять resource rows только с текущим поколением сметы.
- Поле `generation` сейчас защищает от смешения результатов, но история не сохраняется:
  cancel/full rebuild удаляет прежние `estimate_calc_*` строки данной сметы.
- `status='done'` означает, что все обрабатываемые строки терминальны; `lines_errors` может быть больше нуля.
- Нужно заранее определить продуктовую политику: скрывать смету с ошибками, показывать частичный результат или показывать результат с предупреждением.
- Для одного HTTP-ответа желательно читать состояние и агрегаты в транзакции `REPEATABLE READ`, чтобы не смешать поколения.
- `estimate_calc_resources` обновляется транзакционно вместе с завершением строки worker-а.

## Известные ограничения текущего контракта

1. Таблицы аналитики ресурсов пока не имеют `company_id`; tenant проверяется через join к `app_estimates`/стройке.
2. Нет готового HTTP endpoint аналитики ресурсов по стройке.
3. Нет service-to-service auth.
4. `selling_price`, `transport_cost`, `cargo_class`, `corrections` присутствуют в схеме, но текущий GSN snapshot обычно их не заполняет.
5. Пользовательские позиции (`ТПрайс...`, `СТПрайс...`) рассчитываются без GSN resources.
   Поправка `(=N)` после шифра задаёт определитель и создаёт вклад ресурса `(code, determinant=N)`.
   Без `(=...)` snapshot строки может остаться без ресурсных вкладов.
6. Нет тестов persistence/конкурентного обновления `estimate_calc_*`; есть unit-тесты построения `LineCalcSnapshot`.
7. Текущие SQL-индексы оптимизированы в основном под `estimate_id + generation`. После замеров может понадобиться индекс для drill-down по `resource_code`.
8. Для смет, рассчитанных до появления `estimate_calc_*`, возможен только
   legacy `calc_json`; автоматический backfill нормализованных resource tables
   не обнаружен. Такие сметы нужно принудительно пересчитать либо мигрировать.

## Рекомендуемый MVP нового сервиса

1. Backend-only сервис; UI не ходит напрямую в NAV БД/API.
2. Endpoint списка строек с tenant-фильтром.
3. Endpoint состояния готовности аналитики стройки.
4. Endpoint агрегата ресурсов с пагинацией, сортировкой и фильтром.
5. Endpoint drill-down ресурса до сметы/позиции.
6. Команда «обновить расчёты» вызывает NAV API, но не RabbitMQ.
7. Кэшировать только по ключу `(company_id, construction_id, generations[])` или инвалидировать по `estimate_calc_state.updated_at`.
8. Добавить tracing/correlation ID при HTTP-вызовах NAV API.

Минимальные acceptance criteria:

- пользователь не видит стройки другой компании;
- выбор стройки возвращает только её объекты и сметы;
- в аналитику попадают только текущие поколения;
- незавершённые и ошибочные сметы явно обозначены;
- расход ресурса совпадает с суммой `estimate_calc_line_resources`;
- стоимость сметы совпадает с `calc-status?summary=1 -> grandTotal`;
- повторный расчёт не удваивает агрегаты;
- одинаковый `line_id` в разных сметах не вызывает коллизий.

## Решения, которые нужно получить до реализации

1. Новый сервис живёт в этом monorepo или в отдельном репозитории?
2. Нужна аналитика одной компании или superadmin cross-company?
3. Кто инициирует расчёт: пользователь, scheduler или сервис автоматически?
4. Допустим ли частичный результат при `lines_errors > 0`?
5. Какие показатели обязательны: расход, сметная сумма, цена реализации, транспорт, масса?
6. Нужно ли объединять ресурсы с разными `district`/`fgis_set_id`?
7. Нужны ли исторические поколения или только текущее?
8. Требуются ли экспорт, пагинация и поиск по шифру/наименованию?
9. Какой M2M auth будет принят?

## Стартовые точки в коде

- маршруты API: `backend/internal/api/server.go`;
- модели: `backend/internal/domain/models.go`;
- дерево и calc persistence: `backend/internal/store/file_store.go`;
- агрегаты расчёта: `backend/internal/store/estimate_calc_persist.go`;
- worker: `backend/internal/calcworker/worker.go`, `backend/internal/calcworker/rabbit.go`;
- расчёт позиции/ресурсов: `backend/internal/estimatecalc/pricing.go`, `line_result.go`;
- нормативные ресурсы: `backend/internal/gsn/records.go`;
- схема app-БД: `db/schema.sql`, `db/migrate_estimate_calc_tables.sql`;
- схема ГСН/ФГИС: `db/gsn_schema.sql`;
- auth: `backend/internal/auth/session.go`, `backend/internal/api/server.go`;
- окружение: `run.bat`, `scripts/run-calc-worker-exec.bat`;
- эксплуатация: `HANDOFF.md`, `RABBITMQ_MIGRATION_PLAN.md`.

## Готовый промпт для агента-разработчика

> Разработай смежный сервис аналитики ресурсов по стройке, используя `RESOURCE_ANALYTICS_SERVICE_CONTEXT.md` как исходный контракт. Сначала проверь фактическое состояние незакоммиченных calc-таблиц и миграций. Не используй `app_estimates.total`, прямую запись в calc-очередь или агрегацию `calc_json`. Читай стройки и нормализованные результаты из PostgreSQL с обязательным tenant-фильтром, а расчёт запускай и контролируй через NAV API. До кодирования зафиксируй решения по M2M auth, частичным результатам и смешению district/fgis_set_id. Затем предложи минимальный API нового сервиса, SQL-запросы, модель ошибок и тест-план.
