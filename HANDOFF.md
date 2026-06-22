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

**Строка работы** (`isWork`): шифр, «+» → ресурсы. **Позиция-ресурс** (С/М/Т, `resource_catalog`): без «+», стоим. ед. на строке (ФГИС по `fgisSetId`+`district`).

**Ресурс:** номер из нормы → `resource_codifier` → шифр нормы → `records`; **Стоимость ед.** — из `fgis_cs.set_rows` по `fgisSetId`+`district` (ключ `<район>:<значение>/…`): сметная цена или базис + `*индекс` (индекс с новой строки). **Стоимость на объём** = стоим. ед. × расход × объём работы. **Работа** = сумма по ресурсам.

Колонка **Объем** / расход (в шапке). Сессия: `sessionStorage`.

## Код

`web/app.js` · `web/admin.{html,js}` · `backend/internal/{api,store,gsn,presence}/` · `db/{schema,gsn_schema}.sql`

## Админка (`/admin`)

Иконка: `Admin_Icon.ico` · токен: `nav_admin_token` · layout: боковая навигация как в основной системе.

| Раздел | Содержание |
|--------|------------|
| **Пользователи** | CRUD (`/api/users`), компактная таблица, иконки карандаш/корзина; удаление недоступно для суперадмина |
| **Сметы** | Активные lock-сессии (`GET /api/admin/estimate-locks`), кнопки «Обновить» и «Завершить редактирование» (`DELETE /api/admin/estimate-locks/{id}`) |

**Блокировки смет** (`presence.EstimateLocks`, TTL 90 с): пользовательский lock — `PUT/DELETE /api/estimates/{id}/lock`, опрос — `/api/estimate-locks`. Принудительное завершение админом снимает lock и убирает смету из «Редактора» у пользователя; повторное открытие сразу доступно (без блокировки пользователя).

## Ловушки

- Оба PG URL обязательны; после `backend/**` — перезапуск (`.cursor/rules/backend-restart.mdc`).
- PUT сметы с пустым `items` не затирает строки.
- `button { color:#fff }` — цвет текста в списках задавать явно.
