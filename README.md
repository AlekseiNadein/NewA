# NAV SaaS MVP

Первый вертикальный срез пробного проекта по пункту 5 плана и схеме:

`ADMIN FR / USER FR -> BK -> AC/JWT -> storage`, с целевой моделью PostgreSQL.

## Что входит

- Browser UI:
  - экран логина;
  - административный блок для компаний и пользователей;
  - пользовательский блок для CRUD смет.
- Backend API на Go:
  - `/api/auth/login`;
  - `/api/me`;
  - `/api/companies`;
  - `/api/users`;
  - `/api/estimates`.
- Auth/account-контур:
  - пользователи;
  - роли `super_admin`, `company_admin`, `user`;
  - JWT на HMAC SHA-256.
- Предметный модуль:
  - смета;
  - позиции сметы;
  - статусы;
  - пересчет итоговой суммы.
- Storage:
  - временное JSON-хранилище `data/app.json` для быстрого запуска;
  - целевая PostgreSQL-схема в `db/schema.sql`.

## Запуск

```powershell
go run .\backend\cmd\server
```

После запуска открыть:

```text
http://localhost:8080
```

При первом старте автоматически создаются демо-данные.

## Демо-аккаунты

| Роль | Email | Пароль |
| --- | --- | --- |
| `super_admin` | `admin@example.com` | `admin123` |
| `company_admin` | `manager@example.com` | `manager123` |
| `user` | `user@example.com` | `user123` |

## Следующие шаги

1. Заменить `FileStore` на PostgreSQL-реализацию по `db/schema.sql`.
2. Физически вынести `auth/account service` в отдельный процесс, если потребуется строгая сервисная граница.
3. Добавить outbox/queue worker для события изменения сметы.
4. Расширить права с ролей до permissions на действия и сущности.
