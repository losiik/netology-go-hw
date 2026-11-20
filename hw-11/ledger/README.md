# Ledger (hw-11)

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
    ├── service/                  интерфейс Service, бизнес-правила, report.go — конкурентный отчёт, import.go — пакетный импорт
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

## Пакетный импорт

`Service.ImportTransactions(ctx, txs, workers)` — конвейер на каналах:

```
producer ──jobs──> worker 1..N ──results──> collector ──> ImportSummary
```

1. **producer** — горутина, которая по одной отправляет транзакции `{index, tx}` в канал
   `jobs` и закрывает его после последней (или сразу при `ctx.Done()`);
2. **N воркеров** читают `jobs` через `for job := range jobs`, в начале каждой итерации
   проверяют `ctx.Done()` через `select`, затем вызывают `AddTransaction(ctx, tx)` —
   валидация, проверка лимита бюджета, `INSERT` с тем же `ctx`. Результат `{index, err}`
   уходит в `results`;
3. после `wg.Wait()` отдельная горутина закрывает `results`;
4. **collector** — одна горутина, читает `results` до закрытия канала, считает
   `accepted` / `rejected` через `atomic.AddInt64` и добавляет ошибки в срез. Срез пишет
   только коллектор, поэтому гонок нет. После его завершения ошибки сортируются по индексу.

Число воркеров: `0` → `DefaultImportWorkers = 4`, больше `MaxImportWorkers = 32` →
обрезается до 32, больше числа элементов → по числу элементов.

Лимит бюджета: проверка `потрачено + сумма ≤ лимит` и вставка выполняются под мьютексом
**категории**. Транзакции одной категории проходят по очереди, и лимит не превышается
даже при параллельном импорте, а разные категории сохраняются параллельно. Порядок
обработки внутри категории не гарантирован: если две транзакции вместе не влезают в
лимит, отклонена может быть любая из них.

```go
type ImportSummary struct {
	Total    int           // сколько пришло
	Accepted int           // сохранено
	Rejected int           // отклонено (валидация, лимит, ошибка БД)
	Errors   []ImportError // {Index, Error} — индекс элемента во входном массиве
}
```

Ошибки валидации и `budget exceeded` попадают в сводку как есть, ошибки БД — как
`internal error` (подробности в логе).

При отмене или таймауте метод возвращает частичную сводку и ошибку, которая оборачивает
`context.DeadlineExceeded` / `context.Canceled`. Элементы, которые не успели обработать,
не считаются ни принятыми, ни отклонёнными: `accepted + rejected < total`.
Если дедлайн наступил в момент выполнения `INSERT`, строка может успеть сохраниться в БД,
хотя в `accepted` не попадёт — импорт не выполняется одной транзакцией БД.

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

Из папки `hw-11` (PostgreSQL и Redis — тот же `docker-compose.yml`, что в hw-7):

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp?sslmode=disable"
goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
cd ledger
go run ./cmd/ledger
```

## Docker

`Dockerfile` двухэтапный: `golang:1.27-alpine` собирает `./cmd/ledger`, `alpine:latest`
запускает. В образ также кладутся `migrations/` и `budgets.json`. В compose ledger пока
не запускается — Gateway использует его как библиотеку.

```bash
docker build -t cashcraft-ledger ./ledger
docker run --rm   -e DATABASE_URL="postgres://postgres:postgres@host.docker.internal:5434/cashapp?sslmode=disable"   -e REDIS_ADDR=host.docker.internal:6379   cashcraft-ledger
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
| `internal/service`     | 95%      |
| `internal/app`         | 85%      |
| `internal/repository/pg` | 88%    |
