# Gateway (hw-8)

HTTP API на `:8080`. Эндпоинты, DTO и формат ошибок не изменились с hw-7.

## Инициализация сервиса

Gateway не знает про базу данных: он не импортирует `database/sql`, `pgx` и
репозитории, только пакет `ledger` (интерфейс `ledger.Service` и фабрику).

```go
svc, closeFn, err := ledger.New(context.Background())
if err != nil {
	log.Fatal("не удалось инициализировать ledger: ", err)
}
defer closeFn()

http.ListenAndServe(":8080", api.NewRouter(svc))
```

`api.NewRouter(svc)` создаёт `api.Handler` с этим сервисом, обработчики вызывают
методы `svc`. Ошибки сервиса переводятся в HTTP-коды в одном месте (`writeServiceError`):

| Ошибка                     | Код |
|----------------------------|-----|
| неверный JSON              | 400 |
| `ledger.ErrValidation`     | 400 |
| `ledger.ErrBudgetExceeded` | 409 |
| прочие                     | 500 (текст ошибки клиенту не отдаётся, пишется в лог) |

## Запуск

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp?sslmode=disable"
cd hw-8/gateway
go run .
```

## Тесты

```bash
go test ./... -cover
```

Тесты используют `ledger.NewInMemory()`: у каждого теста свой сервис в памяти, БД не нужна.
Ошибки 500 проверяются фейковой реализацией `ledger.Service`. Покрытие `internal/api` — 99%.
