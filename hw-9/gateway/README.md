# Gateway (hw-9)

HTTP API на `:8080`. Эндпоинты транзакций и бюджетов не изменились с hw-8.

## Инициализация сервиса

Gateway не знает про базу данных: он не импортирует `database/sql`, `pgx` и
репозитории, только пакет `ledger` (интерфейс `ledger.Service` и фабрику).

```go
timeout, err := api.TimeoutFromEnv()
...
svc, closeFn, err := ledger.New(context.Background())
if err != nil {
	log.Fatal("не удалось инициализировать ledger: ", err)
}
defer closeFn()

http.ListenAndServe(":8080", api.NewRouter(svc, timeout))
```

## Отчёт по категориям

```
GET /api/reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD
```

Обе даты обязательны и входят в период. Суммы считает Ledger, каждую категорию в
отдельной горутине.

Пример:

```bash
curl -i "http://localhost:8080/api/reports/summary?from=2025-09-01&to=2025-09-30"
```

```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{"from":"2025-09-01","to":"2025-09-30","total":2123.56,"categories":{"еда":1334.56,"транспорт":789}}
```

Если расходов за период нет — `"total":0,"categories":{}`.

Ошибки:

| Ситуация                                  | Код | Тело |
|-------------------------------------------|-----|------|
| нет `from`/`to`, неверный формат, `from > to` | 400 | `{"error":"неверный период отчёта: from=\"2025-13-01\", нужен формат ГГГГ-ММ-ДД"}` |
| превышен таймаут / запрос отменён         | 504 | `{"error":"request timeout"}` |
| ошибка БД                                 | 500 | `{"error":"internal server error"}` |

## Таймауты запросов

Middleware `api.Timeout(d)` оборачивает все `/api/*` ручки: создаёт
`context.WithTimeout(r.Context(), d)` и передаёт обработчику `r.WithContext(ctx)`.
Этот контекст уходит в сервис и дальше во все SQL-запросы (`QueryContext`/`ExecContext`),
поэтому по истечении таймаута запрос к БД прерывается.

- **По умолчанию:** 2 секунды (`api.DefaultRequestTimeout`).
- **Как изменить:** переменная окружения `REQUEST_TIMEOUT` в формате Go duration:
  `500ms`, `5s`, `1m`. Неверное или неположительное значение — ошибка при старте.
- **При превышении:** `504 Gateway Timeout` и `{"error":"request timeout"}`.
  Тот же ответ, если клиент закрыл соединение (`context.Canceled`).

Проверка таймаута:

```bash
REQUEST_TIMEOUT=1ms go run .
curl -i "http://localhost:8080/api/reports/summary?from=2025-09-01&to=2025-09-30"
```

```
HTTP/1.1 504 Gateway Timeout
{"error":"request timeout"}
```

В логе сервера:

```
heartbeat 2025-09-01..2025-09-30: тикер остановлен
расчёт отчёта прерван: context deadline exceeded
ошибка построения отчёта: запрос отменён: context deadline exceeded
GET /reports/summary: превышен таймаут 1ms
```

Ошибки сервиса переводятся в HTTP-коды в одном месте (`writeServiceError`):

| Ошибка                                      | Код |
|---------------------------------------------|-----|
| неверный JSON                               | 400 |
| `ledger.ErrValidation`                      | 400 |
| `ledger.ErrBudgetExceeded`                  | 409 |
| `context.DeadlineExceeded`, `context.Canceled` | 504 |
| прочие                                      | 500 (текст ошибки клиенту не отдаётся, пишется в лог) |

## Запуск

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp?sslmode=disable"
cd hw-9/gateway
go run .
```

## Тесты

```bash
go test ./... -cover
```

Тесты используют `ledger.NewInMemory()` и фейковые реализации `ledger.Service`
(медленный сервис, который ждёт `ctx.Done()`, для проверки 504). Покрытие `internal/api` — 99%.
