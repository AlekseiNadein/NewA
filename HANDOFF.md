# Handoff — NAV SaaS MVP

План: `06_10_структурированный_контекст_пробного_проекта.md`.

## Запуск

`run.bat` → `:8080` · **Система** / **Суперадминистратор** / **admin123**

| Env | Назначение |
|-----|------------|
| `APP_DATA_PATH` | users, companies |
| `APP_DATABASE_URL` | стройки, объекты, сметы, строки |
| `APP_GSN_DATABASE_URL` | `gsn.*`, `fgis_cs.*` |

Импорт справочников (не при старте): `import_regions`, `import_resource_codifier`, `import_fgis_cs`.

## API

Auth · CRUD строек/объектов/смет · GSN: `supplements`, `hierarchy`, `regions`, `record?code&fgisSet&district`, `hierarchy-records`, `fgis-sets`, `fgis-rows`

## Редактор сметы

**Шапка:** шифр, наименование, сметный район (`district`, формат `14.3`), набор ФГИС (`fgisSetId`), сметная стоимость.

**Строка работы:** шифр из `records.original_code`; «+» → ресурсы.

**Ресурс:** номер из нормы → `resource_codifier` → шифр нормы → `records`; **Стоимость ед.** — из `fgis_cs.set_rows` по `fgisSetId`+`district` (ключ `<район>:<значение>/…`): сметная цена или базис + `*индекс` (индекс с новой строки). **Стоимость на объём** = стоим. ед. × расход × объём работы. **Работа** = сумма по ресурсам.

Колонка **Объем** / расход (в шапке). Сессия: `sessionStorage`.

## Код

`web/app.js` · `backend/internal/{api,store,gsn}/` · `db/{schema,gsn_schema}.sql`

## Ловушки

- Оба PG URL обязательны; после `backend/**` — перезапуск (`.cursor/rules/backend-restart.mdc`).
- PUT сметы с пустым `items` не затирает строки.
- `button { color:#fff }` — цвет текста в списках задавать явно.
