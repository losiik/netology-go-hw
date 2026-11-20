# Gateway (hw-7)

HTTP API на `:8080`. Gateway импортирует модуль `ledger` (`replace ledger => ../ledger`)
и при старте вызывает `ledger.Init`, поэтому ему нужны те же переменные окружения,
что и ledger: `DATABASE_URL` (или `DB_*`), `REDIS_ADDR`, `REDIS_DB`, `REDIS_PASSWORD`.

## Запуск

Перед первым запуском: поднять PostgreSQL и Redis и применить миграции
(см. [README ledger](../ledger/README.md)).

```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp?sslmode=disable"
cd hw-7/gateway
go run .
```

## Эндпоинты

| Метод | Путь                                       | Ответ                 |
|-------|--------------------------------------------|-----------------------|
| GET   | `/ping`                                    | 200 `"pong"`          |
| POST  | `/api/budgets`                             | 201 / 400 / 500       |
| GET   | `/api/budgets`                             | 200 / 500             |
| POST  | `/api/transactions`                        | 201 / 400 / 409 / 500 |
| GET   | `/api/transactions`                        | 200 / 500             |
| GET   | `/api/reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD` | 200 / 400 / 500 |

Ошибки — `{"error":"..."}`, заголовок `Content-Type: application/json; charset=utf-8`.

Отчёт:

```json
{"from":"2025-09-01","to":"2025-09-30","total":1250,
 "categories":[{"category":"food","total":950},{"category":"taxi","total":300}]}
```

## Примеры cURL

```bash
curl -X POST http://localhost:8080/api/budgets \
  -H "Content-Type: application/json" \
  -d '{"category":"food","limit":1000}'

curl -X POST http://localhost:8080/api/transactions \
  -H "Content-Type: application/json" \
  -d '{"amount":450,"category":"food","description":"lunch","date":"2025-09-10"}'

curl http://localhost:8080/api/transactions
curl http://localhost:8080/api/budgets

# два раза подряд: в логе сначала cache miss, потом cache hit
curl "http://localhost:8080/api/reports/summary?from=2025-09-01&to=2025-09-30"
curl "http://localhost:8080/api/reports/summary?from=2025-09-01&to=2025-09-30"
```

## Тесты

```bash
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp_test?sslmode=disable"
go test ./... -cover
```

Без `TEST_DATABASE_URL` выполняются только тесты маршрутизации, остальные пропускаются.
