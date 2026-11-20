# Ledger (hw-9)

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
    ├── service/                  интерфейс Service, бизнес-правила, report.go — конкурентный отчёт
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
  - `ExpenseRepository`: `Add`, `List`, `SumByCategory`, `Categories`, `SumByCategoryPeriod`;
  - `Cache`: `Get`, `Set`, `Delete`.
- **repository/pg** реализует интерфейсы домена SQL-запросами. Про HTTP и gateway не знает.
- **service** работает только через интерфейсы домена. В нём валидация, проверка лимита
  (`потрачено + сумма ≤ лимит`, считается в копейках, чтобы не было ошибок округления
  float) и кеширование.
- **app** — единственное место, которое знает про PostgreSQL и Redis и собирает сервис.
- **ledger.go** — фасад для gateway. Нужен потому, что пакеты из `internal/` нельзя
  импортировать из другого модуля, а gateway — отдельный модуль.

## Отчёт по категориям

`Service.GetReportSummary(ctx, from, to)` считает суммы расходов по категориям за период:

1. проверяет даты (`ГГГГ-ММ-ДД`, `from <= to`), иначе `ErrValidation`;
2. смотрит кеш Redis `report:summary:<from>:<to>` (TTL 30s);
3. запускает heartbeat — горутину с `time.Ticker` (`HeartbeatInterval = 300ms`), которая пишет
   в лог `heartbeat ...: идёт расчёт отчёта`;
4. получает список категорий: `SELECT DISTINCT category FROM expenses WHERE date BETWEEN $1 AND $2`;
5. для каждой категории запускает горутину, которая считает
   `SELECT COALESCE(SUM(amount), 0) ... WHERE category = $1 AND date BETWEEN $2 AND $3`.
   Завершение ждём через `sync.WaitGroup`. Каждая горутина пишет только в свою ячейку
   срезов `sums[i]` / `errs[i]`, общий `map` собирается уже после `wg.Wait()`, поэтому гонок нет;
6. останавливает тикер (`stop()` отменяет контекст тикера и ждёт выхода горутины).

Отмена: все запросы к БД получают `ctx` из аргумента (`QueryContext`, `QueryRowContext`,
`ExecContext`). Если `ctx` отменён или истёк дедлайн, горутины не начинают запрос или
прерывают его, метод возвращает `context.DeadlineExceeded` / `context.Canceled`, а в логе:

```
heartbeat 2025-09-01..2025-09-30: тикер остановлен
расчёт отчёта прерван: context deadline exceeded
```

Если одна категория упала с ошибкой БД, остальные горутины отменяются, а наружу уходит
исходная ошибка.

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

Из папки `hw-9` (PostgreSQL и Redis — тот же `docker-compose.yml`, что в hw-7):

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
| `internal/service`     | 93%      |
| `internal/app`         | 85%      |
| `internal/repository/pg` | 88%    |
