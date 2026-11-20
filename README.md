# netology-go-hw

Домашние задания по Go.

Требуется Go 1.27+.

## hw-1

Выводит имя пользователя, аргументы командной строки и версию Go.

```bash
go run ./hw-1 arg1 arg2
```

## hw-2

Два отдельных сервиса, у каждого свой `go.mod`.

### Gateway

HTTP-сервер на порту 8080.

```bash
cd hw-2/gateway
go run .
```

Проверка (в другом терминале или в браузере):

```bash
curl http://localhost:8080/ping
```

Ответ: `pong`, статус `200 OK`.

### Ledger

Хранит транзакции в памяти. При запуске добавляет несколько тестовых транзакций
и выводит их список. Транзакция с суммой 0 не добавляется и выводится ошибка.

```bash
cd hw-2/ledger
go run .
```

Пример вывода:

```
Ledger service started
Ошибка: сумма транзакции не может быть 0
Список транзакций:
1 | 2026-11-18 | Еда          |   350.00 | Обед в столовой
2 | 2026-11-19 | Транспорт    |  1200.00 | Проездной на месяц
3 | 2026-11-20 | Развлечения  |   799.99 | Подписка на кино
```

## hw-3

Продолжение hw-2. Gateway без изменений, в Ledger добавлены бюджеты по категориям.

- `SetBudget` добавляет или обновляет бюджет категории.
- `LoadBudgets` загружает бюджеты из JSON (файл `budgets.json`).
- `AddTransaction` не даёт добавить транзакцию, если по категории будет превышен лимит (`budget exceeded`).

Запуск (из папки `ledger`, иначе не найдётся `budgets.json`):

```bash
cd hw-3/ledger
go run .
```

Формат `budgets.json`:

```json
[
  {"category": "Транспорт", "limit": 3000, "period": "2026-11"},
  {"category": "Развлечения", "limit": 1000, "period": "2026-11"}
]
```

В `main` проверяются сценарии: транзакции в пределах лимита, превышение лимита,
трата ровно до лимита, категория без бюджета, сумма 0 и повторная попытка после
увеличения лимита. Транзакции с отказом в итоговый список не попадают.

Gateway запускается так же, как в hw-2:

```bash
cd hw-3/gateway
go run .
```

## hw-4

Продолжение hw-3. В Ledger добавлена валидация через интерфейс `Validatable`.

- `Validatable` — интерфейс с методом `Validate() error` (`validate.go`).
- `Transaction.Validate()`: сумма > 0, категория не пустая, дата в формате `ГГГГ-ММ-ДД`.
- `Budget.Validate()`: лимит > 0, категория не пустая.
- `AddTransaction`, `SetBudget` и `LoadBudgets` вызывают `Validate()` перед добавлением
  и возвращают ошибку, если данные неверные.
- `CheckValid(v Validatable)` принимает и `Transaction`, и `Budget`.

Запуск:

```bash
cd hw-4/ledger
go run .
```

Что проверить в выводе:

1. Блок `Проверка CheckValid` — для `main.Transaction` и `main.Budget` вызывается одна
   и та же функция, корректные объекты дают `ok`, некорректные — ошибку с причиной.
2. Бюджет с лимитом `-100` не устанавливается (`Отказ: бюджет "Подарки"`).
3. Тесты 1–4 и 6 работают как в hw-3 (лимиты, `budget exceeded`).
4. Тест 5 — транзакции с суммой 0, отрицательной суммой, без категории и с неверной
   датой отклоняются с `невалидная транзакция: ...`.
5. В итоговом списке 6 транзакций, отклонённых там нет.

## hw-5

REST API в Gateway. Gateway импортирует модуль `ledger` из этого же репозитория
(в `gateway/go.mod` указаны `require ledger v0.0.0` и `replace ledger => ../ledger`).
Данные хранятся в памяти и пропадают после перезапуска.

Структура:

```
hw-5/
├── gateway/
│   ├── main.go              запуск сервера на :8080
│   └── internal/api/
│       ├── dto.go           запросы/ответы и преобразование в ledger.Transaction / ledger.Budget
│       ├── handlers.go      обработчики
│       ├── middleware.go    логирование и Content-Type
│       └── router.go        маршруты
└── ledger/
    ├── *.go                 пакет ledger (транзакции, бюджеты, валидация)
    └── cmd/ledger/main.go   консольная демонстрация из hw-4
```

Запуск:

```bash
cd hw-5/gateway
go run .
```

### Эндпоинты

| Метод | Путь                | Описание                     | Ответ                    |
|-------|---------------------|------------------------------|--------------------------|
| GET   | `/ping`             | проверка сервера             | 200 `"pong"`             |
| POST  | `/api/transactions` | создать транзакцию           | 201 / 400 / 409 / 500    |
| GET   | `/api/transactions` | список транзакций            | 200, массив (или `[]`)   |
| POST  | `/api/budgets`      | создать или обновить бюджет  | 201 / 400 / 500          |
| GET   | `/api/budgets`      | список бюджетов              | 200, массив (или `[]`)   |

Все ответы имеют заголовок `Content-Type: application/json; charset=utf-8`.
Ошибки возвращаются в формате `{"error":"..."}`:

- 400 — невалидные данные или неверный JSON;
- 404 — неизвестный путь;
- 405 — неподдерживаемый метод;
- 409 — превышен бюджет (`{"error":"budget exceeded"}`);
- 500 — прочие внутренние ошибки.

Каждый запрос логируется в консоль: метод, путь, время обработки.

### Примеры cURL

Установить бюджет:

```bash
curl -X POST http://localhost:8080/api/budgets \
  -H "Content-Type: application/json" \
  -d '{"category":"еда","limit":1000}'
```

Добавить транзакцию:

```bash
curl -X POST http://localhost:8080/api/transactions \
  -H "Content-Type: application/json" \
  -d '{"amount":450,"category":"еда","description":"ланч","date":"2025-09-10"}'
```

Превышение бюджета → 409. Выполните запрос ниже два раза: первый пройдёт
(450 + 450 = 900 ≤ 1000), второй вернёт `{"error":"budget exceeded"}` (900 + 450 > 1000):

```bash
curl -i -X POST http://localhost:8080/api/transactions \
  -H "Content-Type: application/json" \
  -d '{"amount":450,"category":"еда","description":"ужин","date":"2025-09-10"}'
```

Невалидная транзакция → 400:

```bash
curl -i -X POST http://localhost:8080/api/transactions \
  -H "Content-Type: application/json" \
  -d '{"amount":0,"category":"еда","date":"2025-09-10"}'
```

Списки:

```bash
curl http://localhost:8080/api/transactions
curl http://localhost:8080/api/budgets
```

> На Windows curl передаёт кириллицу из аргументов не в UTF-8, и в ответе вместо
> «еда» будут знаки вопроса. Чтобы этого не было, положите тело запроса в UTF-8
> файл и отправьте его через `--data-binary @body.json`, либо используйте латиницу.

## hw-6

Тесты для Ledger и Gateway. Код сервисов такой же, как в hw-5, в ledger добавлена
функция `Reset()` (`reset.go`) — очищает транзакции и бюджеты между тестами.

- `ledger/ledger_test.go` — unit-тесты домена: табличные тесты `Validate()` для
  `Transaction` и `Budget` (параллельные), сценарий «бюджет → транзакция в лимите →
  превышение», `LoadBudgets`, `ListBudgets`, `CheckValid`.
- `gateway/internal/api/handlers_test.go` — интеграционные тесты HTTP через
  `httptest`: создание и список бюджетов и транзакций, коды 201/200/400/404/405/409,
  формат `{"error":"..."}`, заголовок `Content-Type`. Каждый подтест начинается
  с `ledger.Reset()`, поэтому они не параллельные.

### Запуск тестов

Тесты запускаются отдельно в каждом модуле:

```bash
cd hw-6/ledger
go test ./... -v
```

```bash
cd hw-6/gateway
go test ./... -v
```

Покрытие и HTML-отчёт (в каждой из папок):

```bash
go test ./... -cover -coverprofile=cover.out
go tool cover -html=cover.out -o cover.html
```

В PowerShell аргументы с точкой нужно взять в кавычки, иначе go их не поймёт:
`go test ./... -cover "-coverprofile=cover.out"` и
`go tool cover "-html=cover.out" -o cover.html`.

Файлы `cover.out` и `cover.html` добавлены в `.gitignore`.

### Покрытие

| Пакет                  | Покрытие |
|------------------------|----------|
| `ledger`               | 98.9%    |
| `gateway/internal/api` | 93.1%    |

Покрытие тестами ~95% для кода с бизнес-логикой и HTTP-слоем. `main` обоих сервисов
(`gateway/main.go` и консольная демонстрация `ledger/cmd/ledger`) тестами не покрыт,
поэтому общий процент по модулям ниже: ledger — 23.4%, gateway — 90.4%. Не покрыты
также ветки 500 в обработчиках (после успешной валидации ledger не возвращает других
ошибок, и вызвать их из теста нельзя) и логирование ошибки записи ответа в `writeJSON`.

## hw-7

Ledger хранит данные в PostgreSQL вместо памяти, отчёты и список бюджетов кешируются
в Redis на 30 секунд. Gateway по-прежнему импортирует ledger как модуль; добавлен
эндпоинт `GET /api/reports/summary`, остальные контракты не изменились.

Подробности: [hw-7/ledger/README.md](hw-7/ledger/README.md) и
[hw-7/gateway/README.md](hw-7/gateway/README.md).

### 1. PostgreSQL и Redis

В `hw-7/docker-compose.yml` описаны PostgreSQL 17 (порт **5433**, пользователь
`postgres/postgres`, базы `cashapp` и `cashapp_test`) и Redis 7 (порт 6379).

```bash
cd hw-7
docker compose up -d
```

### 2. Миграции

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
export DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp?sslmode=disable"
goose -dir ./ledger/migrations postgres "$DATABASE_URL" up
```

Откат: `goose -dir ./ledger/migrations postgres "$DATABASE_URL" down`.

### 3. Запуск

Переменные: `DATABASE_URL` (или `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME`),
`REDIS_ADDR` (по умолчанию `localhost:6379`), `REDIS_DB` (`0`), `REDIS_PASSWORD`.

Из корня репозитория, каждый в своём терминале:

```bash
cd hw-7/ledger && go run ./cmd/ledger     # проверка подключения и вывод данных из БД
```

```bash
cd hw-7/gateway && go run .               # HTTP API на :8080
```

В PowerShell переменная задаётся так:
`$env:DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp?sslmode=disable"`.

### 4. Проверка cURL

```bash
curl -X POST http://localhost:8080/api/budgets -H "Content-Type: application/json" \
  -d '{"category":"food","limit":1000}'
curl -X POST http://localhost:8080/api/transactions -H "Content-Type: application/json" \
  -d '{"amount":450,"category":"food","description":"lunch","date":"2025-09-10"}'
curl -X POST http://localhost:8080/api/transactions -H "Content-Type: application/json" \
  -d '{"amount":600,"category":"food","description":"dinner","date":"2025-09-11"}'   # 409
curl http://localhost:8080/api/transactions
curl http://localhost:8080/api/budgets
curl "http://localhost:8080/api/reports/summary?from=2025-09-01&to=2025-09-30"     # cache miss
curl "http://localhost:8080/api/reports/summary?from=2025-09-01&to=2025-09-30"     # cache hit
```

### 5. Тесты

```bash
goose -dir ./ledger/migrations postgres "postgres://postgres:postgres@localhost:5433/cashapp_test?sslmode=disable" up
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp_test?sslmode=disable"
cd hw-7/ledger && go test ./... -cover
cd hw-7/gateway && go test ./... -cover
```

Покрытие: `ledger` — 81%, `gateway/internal/api` — 88%.

## hw-8

Ledger разделён на слои; Gateway работает через интерфейс сервиса и фабрику.
HTTP-контракты не изменились. Подробно: [hw-8/ledger/README.md](hw-8/ledger/README.md),
[hw-8/gateway/README.md](hw-8/gateway/README.md).

```
hw-8/ledger/
├── ledger.go               фасад: ledger.Service, ledger.New(ctx), типы и ошибки
└── internal/
    ├── domain/             сущности, Validate(), ошибки, интерфейсы репозиториев
    ├── repository/pg/      реализация на PostgreSQL
    ├── repository/memory/  реализация в памяти (тесты)
    ├── service/            бизнес-правила (лимит бюджета, отчёт, кеш)
    └── app/                сборка: env → БД → Redis → репозитории → сервис
```

Зависимости направлены внутрь: domain ни от чего не зависит, repository и service
зависят от domain, app собирает всё вместе, gateway знает только `ledger.Service`
и `ledger.New`.

Инициализация в Gateway:

```go
svc, closeFn, err := ledger.New(ctx)   // читает env, подключает PostgreSQL и Redis
if err != nil { log.Fatal(err) }
defer closeFn()
http.ListenAndServe(":8080", api.NewRouter(svc))
```

Запуск — как в hw-7 (те же контейнеры, миграции и переменные), только из папки `hw-8`:

```bash
cd hw-8/gateway
export DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp?sslmode=disable"
go run .
```

Тесты:

```bash
cd hw-8/ledger && go test ./...
cd hw-8/gateway && go test ./...
```

## hw-9

Конкурентный расчёт отчёта в Ledger и таймаут запросов в Gateway.
Подробно: [hw-9/ledger/README.md](hw-9/ledger/README.md), [hw-9/gateway/README.md](hw-9/gateway/README.md).

- `GET /api/reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD` — суммы расходов по категориям.
  Ledger берёт список категорий за период (`SELECT DISTINCT`), считает сумму каждой
  категории в отдельной горутине (`sync.WaitGroup`), пока идёт расчёт — heartbeat
  в лог раз в 300 мс через `time.Ticker`.
- Middleware `Timeout` ставит на каждый `/api/*` запрос `context.WithTimeout`
  (по умолчанию 2s, меняется через `REQUEST_TIMEOUT`). При превышении — `504` и
  `{"error":"request timeout"}`. Все SQL-запросы используют `QueryContext`/`ExecContext`
  с этим контекстом.

```bash
cd hw-9/gateway
export DATABASE_URL="postgres://postgres:postgres@localhost:5433/cashapp?sslmode=disable"
go run .                         # таймаут 2s
REQUEST_TIMEOUT=1ms go run .     # проверка таймаута
```

## hw-10

Пакетный импорт транзакций: конвейер на каналах в Ledger и эндпоинт в Gateway.
Подробно: [hw-10/ledger/README.md](hw-10/ledger/README.md), [hw-10/gateway/README.md](hw-10/gateway/README.md).

```
POST /api/transactions/bulk?workers=4
```

Тело — JSON-массив транзакций в формате `POST /api/transactions`. `workers` — от 1 до 32,
по умолчанию 4. Ответ `200` со сводкой, даже если часть элементов отклонена:

```json
{"total":5,"accepted":2,"rejected":3,"errors":[{"index":1,"error":"сумма должна быть больше 0, получено -5.00"},{"index":3,"error":"budget exceeded"},{"index":4,"error":"неверный формат даты \"2025-13-01\", нужен ГГГГ-ММ-ДД"}]}
```

При таймауте — `504` с `{"error":"request timeout", ...}` и частичной сводкой.

```bash
curl -i -X POST "http://localhost:8080/api/transactions/bulk?workers=4" \
  -H "Content-Type: application/json" \
  -d '[{"amount":120,"category":"food","date":"2025-09-10"},{"amount":-5,"category":"food","date":"2025-09-10"}]'
```

Ledger: producer → канал `jobs` → N воркеров (`sync.WaitGroup`) → канал `results` →
коллектор (`atomic.AddInt64` для счётчиков, срез ошибок пишет только коллектор).
`ctx` проверяется в воркерах через `select` и передаётся во все запросы к БД.

### Проверка гонок

`-race` требует cgo и gcc. На Windows без gcc тесты гоняются в контейнере golang:

```bash
docker run --rm -v "$(pwd)/hw-10:/src" -v gomodcache:/go/pkg/mod -e CGO_ENABLED=1 golang:1.27 \
  bash -c "cd /src/ledger && go test ./... -race && cd /src/gateway && go test ./... -race"
```

## hw-11

Gateway в Docker, запуск через Docker Compose вместе с PostgreSQL и Redis.
Подробно: [hw-11/gateway/README.md](hw-11/gateway/README.md), [hw-11/ledger/README.md](hw-11/ledger/README.md).

```
hw-11/
├── docker-compose.yml      db (postgres:18), redis, gateway
├── gateway/Dockerfile      golang:1.27-alpine → alpine, порт 8080
└── ledger/Dockerfile       golang:1.27-alpine → alpine (в compose пока не запускается)
```

Сервисы compose:

| Сервис    | Образ / сборка | Порт на хосте | Healthcheck |
|-----------|----------------|---------------|-------------|
| `db`      | `postgres:18-alpine`, том `pgdata` | `5434` → 5432 | `pg_isready` |
| `redis`   | `redis:7-alpine` (кеш отчётов, нужен gateway) | — | `redis-cli ping` |
| `gateway` | `./gateway/Dockerfile` | `8080` → 8080 | `wget -qO- http://localhost:8080/ping` |

Gateway ходит в базу по имени сервиса: `DATABASE_URL=postgres://postgres:postgres@db:5432/cashapp?sslmode=disable`,
`REDIS_ADDR=redis:6379`. `depends_on` с `condition: service_healthy` — gateway стартует
только после того, как db и redis прошли healthcheck.

Порт БД на хосте — `5434`, потому что 5432 занят локальным PostgreSQL, а 5433 — контейнером
из hw-7. Порты меняются переменными `DB_PORT` и `GATEWAY_PORT`.

### Запуск

Из папки `hw-11`. Если gateway уже запущен через `go run`, остановите его — порт 8080 должен быть свободен.

1. PostgreSQL, дождаться `healthy`:
   ```bash
   docker compose up -d db
   docker compose ps          # db ... (healthy)
   ```
2. Миграции Goose с хоста через проброшенный порт:
   ```bash
   goose -dir ./ledger/migrations postgres "postgres://postgres:postgres@localhost:5434/cashapp?sslmode=disable" up
   ```
3. Gateway (redis поднимется автоматически по `depends_on`):
   ```bash
   docker compose up -d --build gateway
   docker compose ps          # gateway ... (healthy)
   ```
4. Проверка изнутри контейнера и снаружи:
   ```bash
   docker compose exec gateway wget -qO- http://localhost:8080/ping
   curl http://localhost:8080/ping
   curl -X POST http://localhost:8080/api/budgets -H "Content-Type: application/json" -d '{"category":"food","limit":1000}'
   curl http://localhost:8080/api/budgets
   curl -X POST http://localhost:8080/api/transactions -H "Content-Type: application/json" -d '{"amount":450,"category":"food","date":"2025-09-10"}'
   curl http://localhost:8080/api/transactions
   curl -X POST "http://localhost:8080/api/transactions/bulk?workers=4" -H "Content-Type: application/json" -d '[{"amount":100,"category":"food","date":"2025-09-12"},{"amount":300,"category":"taxi","date":"2025-09-13"}]'
   curl "http://localhost:8080/api/reports/summary?from=2025-09-01&to=2025-09-30"
   ```

Логи и остановка:

```bash
docker compose logs -f gateway
docker compose down        # данные остаются в томе pgdata
docker compose down -v     # удалить и данные
```
