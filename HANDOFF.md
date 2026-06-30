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
| **Auth service** | `run.bat` → `scripts/restart-auth-server.bat` | `:8081` — login, users, companies, licenses |
| **API + frontend** | `run.bat` (основной процесс в текущем окне) | `:8080` — бизнес-API, UI; auth-маршруты **проксируются** на `:8081` |
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

**Проверка после `run.bat`:** три процесса `nav-auth-server.exe`, `nav-server.exe`, `nav-calc-worker.exe`; порты `:8081`, `:8080`; в логе worker нет `GSN database is not configured`.

Логин (рабочая база): **Система** / **nadein.av@yandex.ru** / **admin123**  
Демо из README (**admin@example.com**) в `data/app.json` может отсутствовать; в форме подставляется из черновика `localStorage`.

| Env | Назначение |
|-----|------------|
| `APP_DATA_PATH` | legacy JSON snapshot: **только** settings (если нет PG); сметы/стройки — в `APP_DATABASE_URL` |
| `APP_AUTH_DATABASE_URL` | **auth-контур**: `auth.companies`, `auth.users`, `auth.company_licenses` (по умолчанию = `APP_DATABASE_URL`) |
| `APP_AUTH_SERVICE_URL` | URL отдельного auth-сервиса, напр. `http://127.0.0.1:8081` (если задан — auth API проксируется с `:8080`) |
| `APP_JWT_SECRET` | общий секрет JWT для app и auth (обязательно одинаковый при раздельных процессах) |
| `APP_DATABASE_URL` | стройки, объекты, сметы, строки, очередь расчёта, `app_settings` |
| `APP_GSN_DATABASE_URL` | `gsn.*`, `fgis_cs.*` |

Импорт справочников (не при старте): `import_regions`, `import_resource_codifier`, `import_fgis_cs`.  
Принудительный импорт users/companies из JSON: `go run .\backend\cmd\migrate_auth` (нужен `APP_AUTH_DATABASE_URL`).

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

## Auth-контур (фаза 4 — запланировано)

- Заменить Go reverse proxy (`APP_AUTH_SERVICE_URL`, `internal/api/proxy.go`) на **nginx** как единую точку входа.
- Пример конфига: `deploy/nginx-phase4.example.conf`.
- App-сервис отдаёт только бизнес-API; auth-маршруты маршрутизирует nginx на `:8081`.

## Auth-контур (фаза 2, 2026-06-30)

- `backend/cmd/auth_server` — отдельный процесс `:8081` (`run-auth.bat`).
- `backend/internal/authapi` — HTTP handlers auth API.
- При `APP_AUTH_SERVICE_URL` основной `:8080` **проксирует** auth-маршруты на auth-сервис; frontend не меняется.
- Запуск: сначала `run-auth.bat`, затем `run.bat` (или без `APP_AUTH_SERVICE_URL` — auth in-process, как в фазе 1).

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
5. **UI** — polling `GET /api/estimates/{id}/calc-status`, не подписка на очередь.

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
  → polling calc-status → apply calcJson.record
  → оригинальный шифр, наименование, ед. изм., ресурсы, стоимость
```

### Очередь `estimate_calc_jobs`

Статусы: `queued` | `leased` | `done` | `failed` | `dead`

- Уникальность: `(estimate_id, line_id, revision)`
- Lease ~45 с; просроченный `leased` снова забирается
- Retry с backoff; после `max_attempts` → `dead`
- Задачи создаются для GSN-позиций (`source=gsn`, непустой `code`) при `upsertEstimateDB`

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

Auth · CRUD строек/объектов/смет · `GET /api/estimates/{id}/calc-status` · `GET/PUT /api/settings`

GSN: `supplements`, `hierarchy`, `regions`, `record?code&fgisSet&district`, `hierarchy-records`, `fgis-sets`, `fgis-rows`

## Редактор сметы

**Режимы:** текстовый (default) · табличный.

**Переключение текст → таблица:** `applyEstimateTextToEstimate` (parse) → `restartEstimateTableCalculation` → `renderEditor` → `persistOpenEstimate` → enqueue jobs → `pollEstimateCalcStatus`. **Прямых запросов к ГСН нет** (`recalculateEstimatePricing` не вызывается).

**Смена района / набора ФГИС в табличном режиме:** `clearGsnLineCalcEnrichment` → `restartEstimateTableCalculation` (счётчики и сметная стоимость с 0) → `renderEditor` → `persistOpenEstimate` → `pollEstimateCalcStatus` → анимация прогресса (как при text→table).

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
- `clearGsnLineCalcEnrichment` — сброс обогащения при смене района / набора ФГИС
- `restartEstimateTableCalculation` — обнуление счётчиков прогресса (позиции, ошибки, сметная стоимость) и `markEstimateCalcAwaitingServer`; вызывается при text→table, смене района и смене набора ФГИС в табличном режиме
- `startCalcProgressAnimation` / `estimateCalcProgressTargets` — анимация счётчиков от 0 до фактических значений по мере polling `calc-status`
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
backend/internal/{api,store,gsn,presence,calcworker}/
backend/internal/store/{quantity_expr,source_data_fields}*.go
backend/cmd/{server,auth_server,calc_worker,migrate_auth}/
db/schema.sql, db/auth_schema.sql, db/gsn_schema.sql
run.bat, run-auth.bat, run-calc-worker.bat
scripts/restart-{auth-server,calc-worker}.bat
```

## Админка (`/admin`)

Иконка: `Admin_Icon.ico` · токен: `nav_admin_token`

| Раздел | Содержание |
|--------|------------|
| **Пользователи** | CRUD |
| **Сметы** | Активные lock-сессии, принудительное завершение |

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
- **Комбо «Сметные цены и индексы»:** не делать полный `mountEditorContent` при polling calc-status и после `loadFGISSets` — только обновление таблицы/опций селекта; remount откладывать при фокусе в шапке.

## Текущая сессия (2026-06-30)

Контекст: вынос auth в отдельный сервис + PG ([транскрипт сессии](38c90377-ea92-47e6-8587-fe20b20621b9)) и последующая стабилизация calc worker.

### Auth: фазы 1–3 (реализовано)

| Фаза | Содержание |
|------|------------|
| **1** | `db/auth_schema.sql`, пакет `authstore`, автоимпорт из `app.json` при пустой auth БД |
| **2** | `backend/cmd/auth_server` (`:8081`), `authapi`, proxy с `:8080` при `APP_AUTH_SERVICE_URL` |
| **3** | users/companies/licenses **только** в `auth.*`; `FileStore` без учёток; JWT с `authorized`; `withAuthorized` по claims |

Ключевые пути: `backend/internal/authstore/`, `backend/internal/authapi/`, `backend/internal/api/proxy.go`, `backend/cmd/migrate_auth/`.

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

1. **Фаза 4 auth:** nginx вместо Go-proxy (`deploy/nginx-phase4.example.conf`).
2. Оптимизация `listRecordResources` (убрать N+1) — если снова упираемся в таймаут 40 с.
3. Очистка/компактификация старых `estimate_calc_jobs` (`dead`, stale revision).
4. В UI «Настройки» — подсказка: рекомендуемо 2–4 worker-а.

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
