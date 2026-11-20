# CashCraft — система аналитики личных трат

Микросервисное приложение для учёта личных расходов и бюджетов: транзакции, лимиты по
категориям, отчёты с процентом выполнения бюджета, импорт и экспорт CSV, пакетный импорт,
JWT-авторизация и Google Таблицы в качестве интерфейса.

```
                 HTTP + JWT                    gRPC
Google Таблицы ──────────────┐          ┌──────────────> auth ────┐
(Apps Script)                ▼          │   (регистрация, логин,   │
curl / клиенты ──────────> gateway ─────┤    выдача JWT)           ▼
                       :8080, проверка  │                      PostgreSQL
                       JWT, валидация   └──────────────> ledger ──┤  (budgets, expenses, users)
                                            x-user-id    (бизнес- │
                                                          логика) └──> Redis (кеш отчётов)
```

| Сервис    | Что делает | Порт |
|-----------|------------|------|
| `gateway` | HTTP API, проверка JWT, валидация запросов, преобразование HTTP ↔ gRPC, маппинг ошибок | `8080` (наружу) |
| `ledger`  | транзакции, бюджеты, проверка лимита, отчёты, кеш, CSV, конкурентный импорт | `9090` gRPC (внутри сети) |
| `auth`    | регистрация и логин, bcrypt-хеши паролей, выпуск JWT (HS256) | `9091` gRPC (внутри сети) |
| `db`      | PostgreSQL 18 | `5434` → 5432 |
| `redis`   | кеш отчётов и списка бюджетов | внутри сети |
| `tunnel`  | (по желанию) публичный адрес через Cloudflare Tunnel для Google Таблиц | — |

## Структура репозитория

```
final-hw/
├── docker-compose.yml       все сервисы, healthchecks, depends_on: service_healthy
├── .env.example             настройки и секреты (копируется в .env)
├── proto/                   gRPC-контракты и сгенерированный код (отдельный Go-модуль)
│   ├── ledger/v1/ledger.proto
│   ├── auth/v1/auth.proto
│   └── ledgerv1/, authv1/   *.pb.go (buf generate)
├── ledger/
│   ├── cmd/ledger/          main: gRPC-сервер, health, graceful shutdown
│   ├── migrations/          goose: budgets, expenses, user_id (встроены в бинарь через embed)
│   └── internal/
│       ├── domain/          сущности, валидация, ошибки, интерфейсы репозиториев и кеша
│       ├── service/         бизнес-правила: лимит, отчёт, кеш, CSV, worker-pool импорта
│       ├── repository/pg/   PostgreSQL
│       ├── repository/memory/ в памяти (для тестов)
│       ├── cache/           Redis
│       ├── grpcserver/      транспорт: proto ↔ domain, ошибки → gRPC-коды, x-user-id
│       ├── app/             сборка зависимостей, миграции
│       └── db/              подключение к PostgreSQL
├── auth/                    та же раскладка: domain, service, token (JWT), repository, grpcserver, app
├── gateway/
│   ├── main.go              gRPC-клиенты, HTTP-сервер, graceful shutdown
│   └── internal/
│       ├── api/             роутер, хендлеры, DTO, middleware (логи, таймаут, JWT), gRPC → HTTP
│       ├── auth/            проверка JWT, передача user id в gRPC-метаданных
│       └── config/          настройки из env
├── sheets/                  Google Apps Script: Code.gs, appsscript.json
├── examples/bank_export.csv пример банковской выгрузки для импорта
└── docker/init-test-db.sql  создаёт базу cashapp_test для интеграционных тестов
```

### Чистая архитектура

Зависимости направлены внутрь, к домену:

```
grpcserver ─> service ─> domain <─ repository/pg, repository/memory, cache
     ▲                                   ▲
     └────────────── app (сборка) ───────┘
```

- `domain` ничего не импортирует из проекта: сущности, правила валидации, ошибки и интерфейсы
  `BudgetRepository`, `ExpenseRepository`, `Cache` (в auth — `UserRepository`, `TokenIssuer`).
- `service` работает только через эти интерфейсы, поэтому тестируется на репозиториях в памяти.
- `grpcserver` — транспорт: переводит proto в доменные типы и доменные ошибки в gRPC-коды.
- `app` — единственное место, где создаются PostgreSQL, Redis и конкретные репозитории.
- Gateway не знает про базу данных, он импортирует только сгенерированные gRPC-клиенты из `proto`.

## Быстрый старт

Нужен Docker Desktop. Из папки `final-hw`:

```bash
cp .env.example .env          # затем поменяйте JWT_SECRET на длинную случайную строку
docker compose up -d --build
docker compose ps             # все сервисы должны быть (healthy)
curl http://localhost:8080/ping
# "pong"
```

PowerShell:

```powershell
Copy-Item .env.example .env
docker compose up -d --build
curl.exe http://localhost:8080/ping
```

Миграции применяются автоматически при старте `ledger` и `auth` (goose, `MIGRATE=true`).
Данные лежат в томе `pgdata` и переживают `docker compose down` / `up`
(`docker compose down -v` удаляет и их).

Ручной запуск миграций (если `MIGRATE=false`):

```bash
goose -dir ./ledger/migrations postgres "postgres://postgres:postgres@localhost:5434/cashapp?sslmode=disable" up
goose -dir ./auth/migrations -table auth_goose_db_version postgres "postgres://postgres:postgres@localhost:5434/cashapp?sslmode=disable" up
```

## Конфигурация (.env)

Все секреты и настройки читаются из переменных окружения, в коде их нет.

| Переменная | По умолчанию | Сервис | Описание |
|------------|--------------|--------|----------|
| `JWT_SECRET` | — (обязательна, ≥ 32 символов) | auth, gateway | секрет подписи HS256, общий для выпуска и проверки токена |
| `JWT_TTL` | `24h` | auth | время жизни токена |
| `REQUEST_TIMEOUT` | `2s` в коде, `10s` в `.env.example` | gateway | таймаут на один HTTP-запрос, при превышении — `504` |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | `postgres` / `postgres` / `cashapp` | db, ledger, auth | учётные данные БД |
| `GATEWAY_PORT` | `8080` | compose | порт gateway на хосте |
| `DB_PORT` | `5434` | compose | порт PostgreSQL на хосте |
| `DATABASE_URL` | собирается compose | ledger, auth | строка подключения |
| `REDIS_ADDR` | `redis:6379` | ledger | адрес Redis |
| `LEDGER_ADDR` / `AUTH_ADDR` | `ledger:9090` / `auth:9091` | gateway | адреса gRPC-сервисов |
| `MIGRATE` | `true` | ledger, auth | применять миграции при старте |

## HTTP API

Все ответы — JSON (кроме экспорта CSV). Ошибки в едином формате:

```json
{"error": "описание ошибки"}
```

| Метод | Путь | Авторизация | Описание | Успех |
|-------|------|-------------|----------|-------|
| GET  | `/ping` | нет | проверка, что gateway жив | 200 `"pong"` |
| POST | `/api/auth/register` | нет | регистрация `{login, password}` | 201 |
| POST | `/api/auth/login` | нет | вход, выдаёт JWT | 200 |
| POST | `/api/budgets` | JWT | создать или обновить бюджет категории (upsert) | 201 |
| GET  | `/api/budgets` | JWT | список бюджетов | 200 |
| POST | `/api/transactions` | JWT | добавить транзакцию, проверка лимита | 201 |
| GET  | `/api/transactions` | JWT | список транзакций | 200 |
| POST | `/api/transactions/bulk?workers=4` | JWT | пакетный импорт JSON-массива | 200 |
| POST | `/api/transactions/import?workers=4` | JWT | импорт CSV (тело или multipart-поле `file`) | 200 |
| GET  | `/api/transactions/export.csv?from=&to=` | JWT | выгрузка в CSV (период по желанию) | 200 |
| GET  | `/api/reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD` | JWT | отчёт по категориям | 200 |

Коды ошибок:

| Код | Когда |
|-----|-------|
| 400 | неверный JSON, неизвестное поле, ошибка валидации (сумма ≤ 0, пустая категория, дата не `ГГГГ-ММ-ДД`, неверный период, `workers` вне 1..32) |
| 401 | нет заголовка `Authorization: Bearer <token>`, токен неверный или просрочен, неверный логин/пароль |
| 404 | неизвестный путь |
| 405 | неверный HTTP-метод |
| 409 | превышен лимит бюджета (`budget exceeded`), логин уже занят |
| 413 | тело больше лимита (JSON 1 МБ, bulk и CSV 8 МБ) |
| 500 | внутренняя ошибка (подробности только в логах, клиенту — `internal server error`) |
| 503 | ledger или auth недоступен |
| 504 | превышен `REQUEST_TIMEOUT` |

### Примеры cURL

Регистрация и вход:

```bash
curl -i -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" -d '{"login":"ivan","password":"password123"}'
# 201 {"id":1,"login":"ivan"}

curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" -d '{"login":"ivan","password":"password123"}'
# 200 {"token":"eyJhbGciOiJIUzI1NiIs...","token_type":"Bearer","expires_at":"2025-09-11T12:00:00Z"}

TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login -H "Content-Type: application/json" \
  -d '{"login":"ivan","password":"password123"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')
```

Защищённый API без токена:

```bash
curl -i http://localhost:8080/api/budgets
# 401 {"error":"требуется авторизация: заголовок Authorization: Bearer <token>"}
```

Бюджеты (повторный POST той же категории обновляет лимит):

```bash
curl -i -X POST http://localhost:8080/api/budgets -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" -d '{"category":"food","limit":5000}'
# 201 {"category":"food","limit":5000}

curl -s http://localhost:8080/api/budgets -H "Authorization: Bearer $TOKEN"
# [{"category":"food","limit":5000}]
```

Транзакции:

```bash
curl -i -X POST http://localhost:8080/api/transactions -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"amount":450,"category":"food","description":"lunch","date":"2025-09-10"}'
# 201 {"id":1,"amount":450,"category":"food","description":"lunch","date":"2025-09-10"}

curl -i -X POST http://localhost:8080/api/transactions -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" -d '{"amount":9999,"category":"food","date":"2025-09-11"}'
# 409 {"error":"budget exceeded"}   — запись не создаётся

curl -i -X POST http://localhost:8080/api/transactions -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" -d '{"amount":-5,"category":"food","date":"2025-09-11"}'
# 400 {"error":"сумма должна быть больше 0, получено -5.00"}

curl -s http://localhost:8080/api/transactions -H "Authorization: Bearer $TOKEN"
```

Отчёт:

```bash
curl -s "http://localhost:8080/api/reports/summary?from=2025-09-01&to=2025-09-30" \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "from": "2025-09-01",
  "to": "2025-09-30",
  "total": 2090.5,
  "categories": [
    {"category": "food", "total": 1650.5, "budget": 5000, "budget_percent": 33.01},
    {"category": "taxi", "total": 440, "budget": 1000, "budget_percent": 44},
    {"category": "cinema", "total": 0, "budget": 300, "budget_percent": 0}
  ]
}
```

`budget` и `budget_percent` есть только у категорий с бюджетом. Категории с бюджетом, но без
трат за период, выводятся с `total: 0`.

Пакетный импорт:

```bash
curl -s -X POST "http://localhost:8080/api/transactions/bulk?workers=4" -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '[{"amount":120,"category":"cinema","date":"2025-09-20"},{"amount":-1,"category":"food","date":"2025-09-21"}]'
# 200 {"total":2,"accepted":1,"rejected":1,"errors":[{"index":1,"error":"сумма должна быть больше 0, получено -1.00"}]}
```

Импорт CSV (сырое тело или файл формой):

```bash
curl -s -X POST "http://localhost:8080/api/transactions/import?workers=4" -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: text/csv" --data-binary @examples/bank_export.csv

curl -s -X POST http://localhost:8080/api/transactions/import -H "Authorization: Bearer $TOKEN" \
  -F "file=@examples/bank_export.csv"
```

```json
{"total":7,"accepted":5,"rejected":2,"errors":[
  {"index":3,"line":5,"error":"неверная сумма \"abc\""},
  {"index":4,"line":6,"error":"сумма должна быть больше 0, получено -100.00"}]}
```

Экспорт CSV:

```bash
curl -s -o transactions.csv "http://localhost:8080/api/transactions/export.csv?from=2025-09-01&to=2025-09-30" \
  -H "Authorization: Bearer $TOKEN"
```

```
id,date,amount,category,description
1,2025-09-10,450.00,food,lunch
```

Из PowerShell используйте `curl.exe`, кавычки внутри JSON экранируются `\"`:

```powershell
$login = curl.exe -s -X POST http://localhost:8080/api/auth/login -H "Content-Type: application/json" -d '{\"login\":\"ivan\",\"password\":\"password123\"}' | ConvertFrom-Json
$TOKEN = $login.token
curl.exe -s http://localhost:8080/api/budgets -H "Authorization: Bearer $TOKEN"
```

> В Git Bash на Windows кириллица в аргументе `-d` портится (уходит не в UTF-8). Для русских
> категорий передавайте JSON из файла: `--data-binary @body.json`.

## Бизнес-правила

**Пользователи.** Каждый пользователь видит только свои бюджеты, транзакции и отчёты.
Gateway берёт id из JWT (`sub`) и передаёт его в ledger в gRPC-метаданных `x-user-id`.
Ledger отклоняет вызовы без него с `Unauthenticated`. Порт ledger наружу не открыт.

**Лимит бюджета.** При добавлении транзакции: `уже потрачено в категории + сумма ≤ лимит`,
иначе `409 budget exceeded`, и запись не создаётся. Деньги сравниваются в копейках, чтобы не
было ошибок округления float. Проверка и вставка идут под мьютексом категории пользователя, так
что параллельные запросы не могут вместе превысить лимит.

**Отчёт.** Суммы по категориям за период считаются параллельно: горутина на категорию,
`sync.WaitGroup`, отмена через `ctx`. Пока идёт расчёт, раз в 300 мс в лог пишется heartbeat.
Отчёт содержит траты по категориям, лимит, процент выполнения бюджета и общий итог.

**Кеш Redis.** Ключи `report:summary:<user>:<from>:<to>` и `budgets:<user>`, TTL 30 секунд.
Любая запись пользователя (транзакция, бюджет, импорт) сбрасывает его кеш отчётов. Cache
hit/miss видно в логах ledger:

```bash
docker compose logs -f ledger
# cache miss: report:summary:1:2025-09-01:2025-09-30
# cache set: report:summary:1:2025-09-01:2025-09-30 (ttl 30s)
# cache hit: report:summary:1:2025-09-01:2025-09-30
```

**Пакетный импорт** (`bulk` и CSV) — конвейер на каналах: producer → канал `jobs` → N воркеров
(`workers`, по умолчанию 4, максимум 32) → канал `results` → коллектор. Счётчики считаются через
`atomic.AddInt64`, срез ошибок пишет только коллектор. Воркеры проверяют `ctx.Done()` перед
каждым заданием. Ответ всегда `200` со сводкой: `index` — номер элемента с 0, для CSV есть ещё
`line` — номер строки в файле. Проверено под `go test -race`.

**Формат CSV для импорта.** Первая строка — заголовок, колонки в любом порядке:
`date`, `amount`, `category`, `description` (необязательная) или по-русски `Дата`, `Сумма`,
`Категория`, `Описание`. Разделитель `,` или `;` (определяется по заголовку). Даты
`2025-09-10` или `10.09.2025`, суммы `1234.56`, `1 234,56`. BOM в начале файла допускается.
До 10 000 строк. Экспорт отдаёт UTF-8 с BOM (чтобы Excel правильно показывал кириллицу)
в формате, который можно загрузить обратно через импорт.

## gRPC

Контракты в [proto/ledger/v1/ledger.proto](proto/ledger/v1/ledger.proto) и
[proto/auth/v1/auth.proto](proto/auth/v1/auth.proto). Код генерируется `buf`:

```bash
go install github.com/bufbuild/buf/cmd/buf@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
cd proto && buf lint && buf generate
```

Ошибки на границе сервисов:

| Ошибка домена | gRPC-код | HTTP |
|---------------|----------|------|
| `ErrValidation` | `InvalidArgument` | 400 |
| `ErrBudgetExceeded` | `FailedPrecondition` | 409 |
| `ErrUserExists` | `AlreadyExists` | 409 |
| `ErrInvalidCredentials`, нет `x-user-id` | `Unauthenticated` | 401 |
| `ErrNotFound` | `NotFound` | 404 |
| `context.DeadlineExceeded` / `Canceled` | `DeadlineExceeded` / `Canceled` | 504 |
| сервис не отвечает | `Unavailable` | 503 |
| всё остальное | `Internal` (текст скрыт) | 500 |

Оба gRPC-сервиса регистрируют стандартный `grpc.health.v1`. Healthcheck в compose вызывает
сам бинарь: `./ledger -health`, `./auth -health`.

## Тесты

Каждый сервис — отдельный Go-модуль. Без БД и Redis выполняются unit-тесты: table-driven тесты
домена, сервисов (на репозиториях в памяти), gRPC-серверов (через `bufconn`) и HTTP-хендлеров
gateway (с фейковыми gRPC-серверами). Интеграционные тесты PostgreSQL и Redis включаются
переменными окружения.

```bash
docker compose up -d db redis
export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5434/cashapp_test?sslmode=disable"

cd ledger  && go test ./... -coverpkg=./... -coverprofile=cover.out && go tool cover -func=cover.out | tail -1
cd ../auth && go test ./... -coverpkg=./... -coverprofile=cover.out && go tool cover -func=cover.out | tail -1
cd ../gateway && go test ./... -coverpkg=./... -coverprofile=cover.out && go tool cover -func=cover.out | tail -1
```

`TEST_REDIS_ADDR=localhost:6379` включает тест Redis-кеша. Он использует базу Redis 15 и
очищает её. Redis из этого compose наружу не проброшен, поэтому тест удобнее запускать
командой из блока `-race` ниже.

PowerShell (аргумент с `=` и точкой нужно взять в кавычки):

```powershell
$env:TEST_DATABASE_URL = "postgres://postgres:postgres@localhost:5434/cashapp_test?sslmode=disable"
cd ledger; go test ./... "-coverpkg=./..." "-coverprofile=cover.out"; go tool cover "-func=cover.out"
```

| Модуль | Покрытие (`-coverpkg=./...`, с БД и Redis) |
|--------|-----------------------|
| ledger | 86.6% |
| gateway | 86.4% |
| auth | 68.3% (большую часть непокрытого составляет `main.go`) |

Проверка гонок требует cgo. На Windows без gcc тесты гоняются в контейнере, в сети compose,
с настоящими PostgreSQL и Redis:

```bash
docker run --rm --network cashcraft_default -v "$(pwd):/src" -v gomodcache:/go/pkg/mod \
  -e CGO_ENABLED=1 \
  -e TEST_DATABASE_URL="postgres://postgres:postgres@db:5432/cashapp_test?sslmode=disable" \
  -e TEST_REDIS_ADDR=redis:6379 \
  golang:1.27 bash -c "for m in ledger auth gateway; do cd /src/\$m && go test ./... -race; done"
```

PowerShell (из папки `final-hw`, стек должен быть запущен):

```powershell
docker run --rm --network cashcraft_default -v "${PWD}:/src" -v gomodcache:/go/pkg/mod -e CGO_ENABLED=1 -e TEST_DATABASE_URL="postgres://postgres:postgres@db:5432/cashapp_test?sslmode=disable" -e TEST_REDIS_ADDR=redis:6379 golang:1.27 bash -c 'for m in ledger auth gateway; do cd /src/$m && go test ./... -race; done'
```

## Google Таблицы

Таблица — основной интерфейс: пользователь вносит транзакции и бюджеты в листы, Apps Script
отправляет их в API, а отчёт и выгрузка загружаются обратно в таблицу.

**Рабочая таблица:** [ССЫЛКА_НА_ТАБЛИЦУ ](https://docs.google.com/spreadsheets/d/14PLeVKajXVZkwqgvxWQhx7uKZIU0IMNTPTbeJm7y1JE/edit?usp=sharing)(доступ на просмотр)

| Транзакции со статусами | Отчёт за период |
|---|---|
| ![Транзакции](docs/sheet-transactions.png) | ![Отчёт](docs/sheet-report.png) |

### 1. Публичный адрес API

Apps Script выполняется на серверах Google и не видит `localhost`, поэтому gateway нужно
открыть наружу. В compose есть сервис `tunnel` (Cloudflare Quick Tunnel, без регистрации):

```bash
docker compose --profile public up -d
docker compose logs tunnel | grep trycloudflare.com
# https://random-words.trycloudflare.com
```

Адрес меняется при каждом перезапуске туннеля. Подойдёт и `ngrok http 8080`. Наружу
открывается только gateway, и все прикладные эндпоинты требуют JWT.

### 2. Подключение скрипта

1. Создайте Google Таблицу → **Расширения → Apps Script**.
2. Скопируйте содержимое [sheets/Code.gs](sheets/Code.gs) в `Code.gs`.
3. **Настройки проекта** → включите «Показывать файл манифеста appsscript.json» и замените его
   содержимое на [sheets/appsscript.json](sheets/appsscript.json).
4. Сохраните, вернитесь в таблицу и обновите страницу — появится меню **CashCraft**.
5. Зарегистрируйте пользователя через API (`POST /api/auth/register`).
6. **CashCraft → Настройки подключения…** — адрес туннеля, логин и пароль. При первом запуске
   Google попросит разрешения: доступ к таблице, внешние запросы, триггеры.
7. **CashCraft → Создать листы** создаёт листы «Транзакции», «Бюджеты», «Отчёт», «Выгрузка».

### 3. Операции

| Пункт меню | Что делает | API |
|------------|------------|-----|
| Отправить новые транзакции | все строки листа «Транзакции» с пустым «Статусом» уходят одним запросом. В «Статус» пишется `✓ сохранено` или текст ошибки (`✗ budget exceeded`) | `POST /api/transactions/bulk` |
| Отправить бюджеты | строки листа «Бюджеты» (Категория, Лимит) | `POST /api/budgets` |
| Загрузить отчёт за период | период из ячеек `B1`/`B2` листа «Отчёт», таблица: категория, потрачено, бюджет, % бюджета, итого | `GET /api/reports/summary` |
| Загрузить все транзакции | лист «Выгрузка» | `GET /api/transactions` |
| Включить автоотправку при вводе | устанавливает триггер onEdit: как только в строке заполнены дата, сумма и категория, она отправляется в API, в таблицу пишутся статус и ID | `POST /api/transactions` |

Лист «Транзакции»: `Дата | Сумма | Категория | Описание | Статус | ID`. Дата — ячейка-дата или
текст `ГГГГ-ММ-ДД`, сумма — число или `1 234,56`.

Токен хранится в свойствах пользователя скрипта (`PropertiesService.getUserProperties`) и
обновляется автоматически: перед истечением срока или при ответе `401`.

## Остановка

```bash
docker compose --profile public down   # остановить всё, данные остаются
docker compose down -v                 # удалить и данные
```
