# Handoff — NAV SaaS MVP

План: `06_10_структурированный_контекст_пробного_проекта.md`.

## Запуск

| Процесс | Команда | Назначение |
|---------|---------|------------|
| **API + frontend** | `run.bat` → `:8080` | Основной backend, UI |
| **Сервис расчёта** | `run-calc-worker.bat` | Отдельный процесс; слушает очередь, считает позиции |

Логин: **Система** / **Суперадминистратор** / **admin123**

| Env | Назначение |
|-----|------------|
| `APP_DATA_PATH` | users, companies, настройки приложения (JSON snapshot) |
| `APP_DATABASE_URL` | стройки, объекты, сметы, строки, очередь расчёта, `app_settings` |
| `APP_GSN_DATABASE_URL` | `gsn.*`, `fgis_cs.*` |

Импорт справочников (не при старте): `import_regions`, `import_resource_codifier`, `import_fgis_cs`.

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
| `run-calc-worker.bat` | Сборка `data\nav-calc-worker.exe` и запуск |

**Основной backend (`backend/cmd/server`) worker не запускает.**

Manager перечитывает `calcWorkerCount` из настроек каждые **5 с** и перестраивает пул goroutine (1–16, default **2**).

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

**Переключение текст → таблица:** `applyEstimateTextToEstimate` (parse) → `renderEditor` → `persistOpenEstimate` → enqueue jobs → `pollEstimateCalcStatus`. **Прямых запросов к ГСН нет** (`recalculateEstimatePricing` не вызывается).

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
backend/cmd/{server,calc_worker}/
db/schema.sql, db/gsn_schema.sql
run.bat, run-calc-worker.bat
```

## Админка (`/admin`)

Иконка: `Admin_Icon.ico` · токен: `nav_admin_token`

| Раздел | Содержание |
|--------|------------|
| **Пользователи** | CRUD |
| **Сметы** | Активные lock-сессии, принудительное завершение |

**Блокировки смет** (`presence.EstimateLocks`, TTL 90 с): `PUT/DELETE /api/estimates/{id}/lock`.

## Ловушки

- Оба PG URL обязательны; после `backend/**` — перезапуск API (`run.bat`); worker — отдельно (`run-calc-worker.bat`).
- Без worker-сервиса задачи копятся в очереди, UI показывает «В очереди».
- PUT сметы с пустым `items` не затирает строки.
- `go test ./backend/internal/store/ -run Quantity` — тесты объёма и фикстур Э10410/Э10420.
- Backup исходников перед правками: `backups/2026-06-27_*` (последние сессии).
- Не показывать `originalCode` до завершения calc — иначе в таблице виден шифр из ГСН вместо исходного из текста.
- `recalculateEstimatePricing` в `web/app.js` оставлена, но **не вызывается**; расчёт только через worker + polling.

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

### Незакоммиченные изменения

`HANDOFF.md`, `backend/internal/store/{quantity_expr,source_data_fields}*.go`, `file_store.go`, `web/app.js`, `Э10410.txt`, `Э10420.txt`, `tools/`

---

## Предыдущая сессия (2026-06-27)

### Сделано

1. **Очередь расчёта** в PostgreSQL + поля calc на строках сметы.
2. **Worker** вынесен из API в `calcworker` + `cmd/calc_worker`.
3. **API** `calc-status`, `settings`.
4. **Frontend:** polling статусов, badges, `rawText` при parse, persist при text→table.
5. **Настройки UI:** количество worker-ов (1–16).
6. **«+ Создать новую смету»** → `POST /api/estimates` + `refreshConstructionData()`; смета сразу в дереве «Стройки» (временно под объектом по умолчанию, пока не разобран текст).
7. **При сохранении сметы** (`persistOpenEstimate`) → `syncEstimateToConstructionTree`: по строке `Ю` создаёт/находит стройку и объект, переносит смету в нужный узел дерева.
8. **Табличный редактор до расчёта:** только исходный шифр (`sourceCode`) + объём; без клиентского fallback к ГСН.
9. **Табличный редактор после расчёта:** оригинальный шифр, наименование, ед. изм., ресурсы, стоимость — из `calc_json` через polling.
10. **Смена района / набора ФГИС:** `clearGsnLineCalcEnrichment` → persist → повторная очередь (без `recalculateEstimatePricing`).
11. **Формат «Исходные данные»:** `F(49)` — только `наборФГИС`, без дублирования в `описание` (смета Э7151769030).

### Проверено на примере

- Смета **Э7151769030** (формат «Исходные данные», `наборФГИС=alrosa-2026-q2`) — сценарий text→table→calc.
- Фикстура **Э10410.txt** в корне — типовой набор GSN-позиций с исходными шифрами вида `Е0624-001-05 (РМ…)`.
