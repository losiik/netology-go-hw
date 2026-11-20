# Ledger (hw-8)

Бизнес-логика, разделённая на слои: domain → repository → service, плюс слой сборки.
Хранение — PostgreSQL, кеш отчётов и списка бюджетов — Redis (TTL 30 секунд).

## Структура

```
ledger/
├── ledger.go                     публичный фасад: интерфейс Service, типы, ошибки, фабрика New
├── cmd/ledger/main.go            проверка подключения и вывод данных из БД
├── migrations/                   SQL-миграции goose (без изменений с hw-7)
└── internal/
    ├── domain/                   сущности, валидация, ошибки, интерфейсы репозиториев и кеша
    ├── repository/
    │   ├── pg/                   реализация репозиториев на PostgreSQL
    │   └── memory/               реализация в памяти (для тестов)
    ├── service/                  интерфейс Service и бизнес-правила
    ├── app/                      сборка: конфиг из env → БД → Redis → репозитории → сервис
    ├── db/                       sql.Open("pgx"), пул, Ping
    └── cache/                    клиент Redis и адаптер под domain.Cache
```

## Слои и зависимости

```
gateway ──> ledger (фасад) ──> app ──> service ──> domain
                                 ├──> repository/pg ──> domain
                                 ├──> repository/memory ──> domain
                                 ├──> cache ──> domain
                                 └──> db
```

- **domain** ни от чего не зависит. Здесь `Transaction`, `Budget`, `ReportSummary`,
  методы `Validate()`, ошибки (`ErrBudgetExceeded`, `ErrValidation`, `ErrNotFound`,
  `ErrInvalidPeriod`) и интерфейсы:
  - `BudgetRepository`: `Upsert`, `GetByCategory`, `List`;
  - `ExpenseRepository`: `Add`, `List`, `SumByCategory`, `SummaryByCategory`;
  - `Cache`: `Get`, `Set`, `Delete`.
- **repository/pg** реализует интерфейсы домена SQL-запросами. Про HTTP и gateway не знает.
- **service** работает только через интерфейсы домена. В нём валидация, проверка лимита
  (`потрачено + сумма ≤ лимит`, считается в копейках, чтобы не было ошибок округления
  float) и кеширование.
- **app** — единственное место, которое знает про PostgreSQL и Redis и собирает сервис.
- **ledger.go** — фасад для gateway. Нужен потому, что пакеты из `internal/` нельзя
  импортировать из другого модуля, а gateway — отдельный модуль.

## Фабрика

```go
svc, closeFn, err := ledger.New(ctx)
```

- читает `DATABASE_URL` (или `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME`)
  и `REDIS_ADDR`, `REDIS_DB`, `REDIS_PASSWORD`;
- открывает БД (`MaxOpenConns=10`, `MaxIdleConns=5`), делает `Ping` с переданным `ctx`;
- подключает Redis, собирает репозитории и сервис;
- возвращает `ledger.Service`, функцию закрытия соединений и ошибку.

Миграции фабрика не запускает. `ledger.NewInMemory()` возвращает сервис на репозиториях
в памяти без кеша — используется в тестах gateway.

## Миграции и запуск

Из папки `hw-8` (PostgreSQL и Redis — тот же `docker-compose.yml`, что в hw-7):

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp?sslmode=disable"
goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
cd ledger
go run ./cmd/ledger
```

## Тесты

```bash
go test ./...
```

Без БД выполняются тесты домена, сервиса (на репозиториях в памяти и фейковом кеше),
конфигурации и фабрики. Тесты `repository/pg` и подключения к БД требуют
`TEST_DATABASE_URL`:

```bash
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp_test?sslmode=disable"
go test ./... -cover
```

| Пакет                  | Покрытие |
|------------------------|----------|
| `internal/domain`      | 100%     |
| `internal/service`     | 89%      |
| `internal/app`         | 85%      |
| `internal/repository/pg` | 85%    |
