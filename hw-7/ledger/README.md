# Ledger (hw-7)

Бизнес-логика: транзакции, бюджеты, отчёты. Данные хранятся в PostgreSQL,
отчёты и список бюджетов кешируются в Redis на 30 секунд.

## Структура

```
ledger/
├── migrations/          SQL-миграции goose
├── internal/db/         подключение к PostgreSQL (pgx), пул соединений
├── internal/cache/      подключение к Redis (go-redis)
├── storage.go           Init / Close / Reset, хелперы кеша
├── transaction.go       AddTransaction, ListTransactions
├── budget.go            SetBudget (UPSERT), ListBudgets, LoadBudgets
├── report.go            GetReportSummary
└── cmd/ledger/main.go   запуск: подключение и вывод данных из БД
```

## Переменные окружения

| Переменная       | По умолчанию     | Описание                                    |
|------------------|------------------|---------------------------------------------|
| `DATABASE_URL`   | —                | строка подключения, имеет приоритет         |
| `DB_HOST`        | `localhost`      | используются, если `DATABASE_URL` не задан  |
| `DB_PORT`        | `5432`           |                                             |
| `DB_USER`        | `postgres`       |                                             |
| `DB_PASS`        | `postgres`       |                                             |
| `DB_NAME`        | `cashapp`        |                                             |
| `REDIS_ADDR`     | `localhost:6379` |                                             |
| `REDIS_DB`       | `0`              |                                             |
| `REDIS_PASSWORD` | —                |                                             |

PostgreSQL из `docker-compose.yml` слушает порт **5433**, поэтому `DATABASE_URL`
нужно задать явно (см. ниже).

Пул соединений: `MaxOpenConns=10`, `MaxIdleConns=5`, `ConnMaxLifetime=30m`.

## Миграции

Установка goose:

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

Применить (из папки `hw-7`):

```bash
goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
```

Откатить последнюю миграцию:

```bash
goose -dir ./ledger/migrations postgres "$DATABASE_URL" down
```

Статус:

```bash
goose -dir ./ledger/migrations postgres "$DATABASE_URL" status
```

Таблицы:

- `budgets` — `id`, `category` (UNIQUE), `limit_amount` (> 0);
- `expenses` — `id`, `amount` (<> 0), `category`, `description`, `date`, индекс по `(category, date)`.

## Запуск

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp?sslmode=disable"
cd hw-7/ledger
go run ./cmd/ledger
```

При старте в лог пишется подключение к PostgreSQL и Redis, затем выводятся бюджеты
и транзакции из БД. Если БД или Redis недоступны — понятная ошибка и выход с кодом 1.

Загрузить бюджеты из файла:

```bash
go run ./cmd/ledger -load budgets.json
```

## Кеш

| Ключ                           | Что хранится          | TTL | Сброс                    |
|--------------------------------|-----------------------|-----|--------------------------|
| `report:summary:<from>:<to>`   | JSON отчёта за период | 30s | только по TTL            |
| `budgets:all`                  | JSON списка бюджетов  | 30s | `DEL` после `SetBudget`  |

В лог пишется `cache miss`, `cache set` и `cache hit`. Отчёт после новой транзакции
может быть устаревшим до 30 секунд — это ожидаемо.

## Тесты

Тесты с БД используют отдельную базу `cashapp_test` и Redis DB 15, их данные
очищаются через `Reset()`. Без `TEST_DATABASE_URL` такие тесты пропускаются.

```bash
goose -dir ./migrations postgres "postgres://postgres:postgres@localhost:5433/cashapp_test?sslmode=disable" up
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp_test?sslmode=disable"
go test ./... -cover
```

Не запускайте тесты ledger и gateway одновременно: они используют одну тестовую базу.
