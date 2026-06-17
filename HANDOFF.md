# Handoff — NAV SaaS MVP (состояние на 15.06.2026)

Компактный контекст для продолжения разработки. Полный план — в `06_10_структурированный_контекст_пробного_проекта.md`.

## 1. Цель проекта

MVP SaaS-платформы на основе предметной области сметного продукта (ГСН-2022):

- browser UI → backend API → auth/JWT → storage;
- компании, пользователи, роли, CRUD по предметным сущностям;
- нормативная база ГСН из PostgreSQL;
- учебный фокус — backend, но frontend обязателен для видимого результата.

Целевая схема из обсуждения «Схема»: `ADMIN FR / USER FR → BK → AC/JWT → PG`.

## 2. Архитектура (текущая реализация)

```
Browser (web/)
    ↓ HTTP + JWT Bearer
Go backend (backend/cmd/server)
    ├── auth      — login, JWT HMAC SHA-256, роли
    ├── store     — FileStore (data/app.json) — SaaS-сущности
    ├── gsn       — PostgreSQL schema gsn.* — нормативная база
    └── static    — раздача web/
```

Два контура данных:

| Контур | Хранилище | Назначение |
|--------|-----------|------------|
| SaaS (компании, пользователи, стройки, сметы) | `data/app.json` (временно) | MVP приложения |
| ГСН (иерархия, записи, поправки, NSI) | PostgreSQL `gsn` schema | Нормативная база |

## 3. Структура репозитория

```
backend/
  cmd/server/main.go          — точка входа
  internal/
    api/server.go             — HTTP routes
    auth/service.go           — JWT, пароли
    domain/models.go            — Company, User, Construction, Estimate...
    store/file_store.go       — JSON persistence
    gsn/service.go            — чтение gsn.* из PostgreSQL
web/
  index.html, app.js, styles.css
db/
  schema.sql                  — целевая SaaS-схема (PostgreSQL)
  gsn_schema.sql              — схема нормативной базы
scripts/
  import_gsn_to_postgres.py   — импорт Books → PostgreSQL
prep_gsn_books.py             — подготовка текстовых файлов из Books
data/
  app.json                    — runtime SaaS-данные
  gsn_postgres_import/        — TSV + load_gsn.sql
  nav-server.exe              — собранный бинарник
run.bat                       — запуск сервера
```

## 4. Запуск

```bat
run.bat
```

Переменные в `run.bat`:

| Переменная | Значение | Назначение |
|------------|----------|------------|
| `APP_ADDR` | `:8080` | Порт |
| `APP_WEB_DIR` | `web` | Статика |
| `APP_DATA_PATH` | `data\app.json` | SaaS JSON-store |
| `APP_GSN_DATABASE_URL` | `user=postgres password=postgres dbname=postgres sslmode=disable` | PostgreSQL ГСН |

Сборка: `go build -o data\nav-server.exe backend\cmd\server`

URL: `http://localhost:8080`

### Демо-аккаунты

| Роль | Email | Пароль |
|------|-------|--------|
| super_admin | admin@example.com | admin123 |
| company_admin | manager@example.com | manager123 |
| user | user@example.com | user123 |

## 5. API (реализовано)

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/api/auth/login` | Логин → JWT |
| GET | `/api/me` | Текущий пользователь |
| GET/POST | `/api/companies` | Компании |
| GET/POST | `/api/users` | Пользователи |
| GET/POST | `/api/constructions` | Стройки (уровень 1) |
| GET/POST | `/api/objects` | Объекты (уровень 2) |
| GET/POST/PUT/DELETE | `/api/estimates` | Сметы (уровень 3) |
| GET | `/api/gsn/base-info` | Редакция СНБ + Версия из `gsn.base_info_params` |
| GET | `/api/gsn/hierarchy?parent=&limit=` | Дочерние узлы иерархии (ленивая загрузка) |

## 6. UI — навигация (реализовано)

Левое меню:

- **База**
  - ГСН-2022 — дерево `gsn.hierarchy`, заголовок из `Редакция СНБ`, версия из `Версия`
  - Позиции пользователя — заглушка
- **Стройки** — CRUD: Стройка → Объект → Смета
- **Редактор**
  - Буфер (всегда) — сессионный, `sessionStorage`
  - Сметы сессии — подразделы по мере создания
  - Кнопка «Создать новую смету»
- **Документы** — заглушка
- **Настройки**
  - «Отображать шифр иерархии» (`localStorage`, по умолчанию выкл.)

### ГСН-2022 — действия на конечных узлах

- **В буфер** — добавить позицию в сессионный буфер редактора
- **В смету** — только если в редакторе открыта ровно одна смета

Доп. информация об уровнях (уровень, ед. изм., нормы) в дереве **не показывается**.

## 7. Данные ГСН (PostgreSQL)

### Источник

`D:\OneDrive\Bases\Разработка\000 - RU ГСН-2022\2026-06-15 Сокращенная база\Books\`

Кодировка исходников: **Windows-1251 (cp1251)**.

### Пайплайн подготовки

1. `prep_gsn_books.py` — очистка текстов, отчёты, `00~hierarchy.clean.txt` и др.
2. `scripts/import_gsn_to_postgres.py` — парсинг → TSV → `load_gsn.sql`
3. `psql` — применение `db/gsn_schema.sql` + `\copy` из TSV

### Ключевые таблицы `gsn` schema

| Таблица | Содержимое |
|---------|------------|
| `base_info` + `base_info_params` | Метаданные базы из `00~f.txt` (30 параметров) |
| `hierarchy` | Дерево разделов из `00~hierarchy.txt` |
| `hierarchy_record_refs` | Ссылки раздел → шифр нормы |
| `records` | Нормативные записи из файлов `00.prf` |
| `record_resources` | Ресурсы внутри записей |
| `nsi` | Доп. характеристики из `00~nsi.txt` |
| `amendments` + `amendment_impacts` | Поправки из `00~ppr.txt` |
| `record_amendments` | Инциденции из `00~amen.txt` (без FK на amendments — 8 missing) |

Представление: `gsn.base_info_json` — все параметры одним JSONB.

### Объёмы (после импорта)

- hierarchy: ~118k строк
- records: ~480k
- base_info_params: 30

## 8. Решения и особенности

- **FileStore password fix**: `passwordHash`/`passwordSalt` хранятся в JSON через `storedUser`, не через domain JSON tags.
- **Auth в коде**, не отдельный процесс — достаточно для MVP.
- **Редактор сессионный** — буфер и сметы только в `sessionStorage`, backend API для редактора пока нет.
- **PNG vs JPG**: агент Cursor не видит `Схема.png`, работает `Схема.jpg`.
- **run.bat на Windows**: рабочая директория через `cd /d`, иначе не находит `web` и `data`.
- **Процесс сервера**: при запуске через Cursor background task `.bat` может завершиться с exit code 1, но дочерний `nav-server.exe` продолжает работать.

## 9. Следующий этап (приоритеты)

### A. Авторизация (см. `Авторизация.md`)

- Вход: компания + ФИО + пароль (не email).
- Регистрация: компания, ФИО, email, пароль, повтор пароля.
- Новый пользователь: `авторизован=false`, `администратор=false`.
- Пока без полноценного approval flow.

### B. Редактор (backend)

- API для буфера и сессионных смет (сейчас только frontend/sessionStorage).
- Добавление позиций из ГСН в смету с разрешением норм (`hierarchy_record_refs` → `records`).
- Перенос из буфера в смету.

### C. SaaS storage

- Заменить `FileStore` на PostgreSQL по `db/schema.sql`.
- Миграция `data/app.json` → PG.

### D. ГСН — углубление

- Просмотр нормативной записи по шифру (из дерева или поиска).
- Поиск по `gsn.records` (trgm-индекс уже есть).
- Позиции пользователя (подраздел Базы).

### E. Инфраструктура (позже)

- Outbox/queue worker (RabbitMQ).
- Отдельный auth-service.
- Docker Compose, K3s deploy.
- Подписки/лицензирование.

## 10. Исходные документы

| Файл | Назначение |
|------|------------|
| `06_10_структурированный_контекст_пробного_проекта.md` | Полный контекст из транскрибации «План» |
| `06_10_Планирование_...txt` | Исходная транскрибация |
| `Схема.jpg` | Архитектурная схема (ADMIN FR, USER FR, BK, AC, PG) |
| `Авторизация.md` | Требования к auth на следующем этапе |
| `Заметки.md` | Рабочие заметки по Cursor/агенту |

## 11. Стек

- **Backend**: Go 1.25, stdlib HTTP, `github.com/jackc/pgx/v5`
- **Frontend**: vanilla HTML/CSS/JS (без сборщика)
- **SaaS DB (цель)**: PostgreSQL (`db/schema.sql`)
- **ГСН DB**: PostgreSQL 16 (`db/gsn_schema.sql`)
- **Импорт**: Python 3 (`prep_gsn_books.py`, `scripts/import_gsn_to_postgres.py`)
