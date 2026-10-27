# trip-service

HTTP API для MVP такси-сервиса: создание поездки, получение поездки, завершение
поездки. Это лабораторные работы №1 и №2 курса «Разработка микросервисов на Go»
(Avito, осень 2026). В первой написан сам сервис, во второй он разложен по
слоям, покрыт тестами и подключён к OpenTelemetry.

## Стек

- Go 1.27
- PostgreSQL
- [chi](https://github.com/go-chi/chi): HTTP-роутер
- [pgx/v5](https://github.com/jackc/pgx): драйвер PostgreSQL
- [squirrel](https://github.com/Masterminds/squirrel): построение SQL-запросов
- [goose](https://github.com/pressly/goose): миграции схемы БД
- [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen): генерация
  серверных типов и интерфейса обработчиков из OpenAPI-контракта
- [OpenTelemetry](https://opentelemetry.io/) (SDK, экспорт по OTLP/HTTP) и
  [otelpgx](https://github.com/exaring/otelpgx): трейсы и метрики
- [testify](https://github.com/stretchr/testify) и
  [gomock](https://github.com/uber-go/mock): тесты
- [golangci-lint](https://golangci-lint.run/): линтер

## Требования

- Go 1.24 или новее (в `go.mod` указан toolchain `1.27.1`)
- Docker (нужен для `tripgoctl`)
- [`tripgoctl`](https://github.com/course-go-autumn-2026/course-infra):
  разворачивает локальные PostgreSQL и стек наблюдаемости (коллектор,
  Prometheus, Grafana, Jaeger)

## Запуск

```bash
tripgoctl cluster start
tripgoctl environment start
set -a; source .env; set +a
make migrate
make run
```

`tripgoctl environment start` создаёт `.env` в корне репозитория с реальными
адресами PostgreSQL и коллектора. Сервис сам `.env` не читает, поэтому в
каждом новом терминале переменные нужно загрузить командой
`set -a; source .env; set +a`. Часть переменных (см. ниже) в этот `.env` не
входит: они не обязательны и при отсутствии принимают значение по умолчанию.

Проверка, что сервис запустился:

```bash
curl -i localhost:8080/health
curl -i localhost:8080/ready
```

Адреса Jaeger, Grafana и Prometheus печатает `tripgoctl connect`.

## Переменные окружения

Обязательные (без них сервис не запускается):

| Переменная | Назначение |
|---|---|
| `HTTP_ADDR` | адрес, на котором слушает HTTP-сервер |
| `LOG_LEVEL` | уровень логирования (`debug`/`info`/`warn`/`error`) |
| `SHUTDOWN_TIMEOUT` | бюджет на graceful shutdown |
| `DATABASE_URL` | строка подключения к PostgreSQL |
| `DATABASE_MAX_CONNS`, `DATABASE_MIN_CONNS` | размер пула соединений |
| `DATABASE_MAX_CONN_LIFETIME` | максимальное время жизни соединения в пуле |
| `DATABASE_CONNECT_TIMEOUT` | таймаут установки соединения с БД |
| `DATABASE_QUERY_TIMEOUT` | таймаут запроса к БД (зарезервировано для будущих repository-методов) |

Переменные OpenTelemetry читает сам SDK, в `config` их разбора нет. Их создаёт
`tripgoctl`:

| Переменная | Назначение |
|---|---|
| `OTEL_SERVICE_NAME` | имя сервиса (`trip-service`) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | адрес коллектора |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | протокол экспорта (`http/protobuf`) |
| `OTEL_RESOURCE_ATTRIBUTES` | дополнительные атрибуты ресурса (`service.namespace`, `deployment.environment`) |
| `OTEL_TRACES_SAMPLER` | сэмплер трейсов (`parentbased_always_on`) |

Необязательные, не входят в базовый набор, который требует эта работа,
поэтому `tripgoctl` их не создаёт. При отсутствии сервис всё равно
запускается со значением по умолчанию; если переменная задана, но её
значение некорректно, запуск завершится ошибкой:

| Переменная | Значение по умолчанию |
|---|---|
| `HTTP_READ_TIMEOUT` | `5s` |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` |
| `HTTP_WRITE_TIMEOUT` | `10s` |
| `HTTP_IDLE_TIMEOUT` | `60s` |

Полный список переменных приведён в `.env.example`.

## Makefile

| Цель | Действие |
|---|---|
| `make migrate` | применить миграции (`goose up`) |
| `make run` | запустить сервис |
| `make generate` | перегенерировать код из OpenAPI-контракта и моки (`go generate`) |
| `make lint` | `golangci-lint` |
| `make test` | unit-тесты: `go test -race -count=2 -cover ./...` |
| `make test-integration` | все тесты, включая интеграционные с PostgreSQL (тег `integration`) |

## API

Реализованы пять операций контракта:

| Метод | Путь | Коды ответа |
|---|---|---|
| `POST` | `/api/v1/trips` | `201`, `200`, `400`, `409` |
| `GET` | `/api/v1/trips/{tripId}` | `200`, `404` |
| `POST` | `/api/v1/trips/{tripId}/finish` | `200`, `404`, `409` |
| `GET` | `/health` | `200` |
| `GET` | `/ready` | `200`, `503` |

Ошибки оформлены как `application/problem+json` (RFC 9457) с полем `code`:
`invalid_request`, `trip_not_found`, `trip_completed`, `driver_busy`,
`idempotency_conflict`, `internal_error`.

### Идемпотентность

`POST /api/v1/trips` принимает необязательный заголовок `Idempotency-Key`
(UUID). Повторный запрос с тем же ключом и тем же телом возвращает уже
созданную поездку с кодом `200`, а не создаёт вторую. Тот же ключ с другим
телом даёт `409 idempotency_conflict`.

## Архитектура

Код лежит в `internal/` и разложен по слоям. На каждое действие (`create`,
`get`, `finish`) заведён свой пакет с одним и тем же именем в `usecase` и в
`http`.

```
cmd/trip-service/             main: читает конфиг, вызывает container.New и запускает результат
api/                          код, сгенерированный из OpenAPI
migrations/                   миграции goose
internal/
  config/                     разбор и проверка переменных окружения
  container/                  создаёт все зависимости и связывает их вручную
  domain/trip/                поездка, статусы, правила, доменные ошибки
  model/                      структуры, которые ходят между слоями
  usecase/trip/{create,get,finish}/
  http/trip/{create,get,finish}/   ручки
  http/domainerror/           единственное место, где ошибка превращается в HTTP-ответ
  http/middleware/            Recover, Tracing, Logging, Metrics
  http/health/                /health и /ready
  http/router/                собирает ручки и middleware в один http.Handler
  repository/trip/            SQL-реализация: поездки и ключи идемпотентности
  infra/postgres/             пул pgx и трассировка запросов
  infra/postgres/transaction/ менеджер транзакций
  infra/telemetry/            OpenTelemetry и slog
```

### Кто от кого зависит

Зависимости направлены внутрь. Домен не импортирует ничего из проекта.
`usecase` знает про домен и `model`. Ручки и репозиторий тоже зависят от
домена и `model`, но не друг от друга: ручка не импортирует `repository` и
`infra`, а `usecase` не импортирует `net/http` и `pgx`. Связывает всё только
`container`. Эти правила записаны в `depguard` в `.golangci.yml`, поэтому
нарушение ломает `make lint`.

### Где объявлены интерфейсы и почему

Интерфейс объявляет тот, кто им пользуется, в файле `contract.go` своего
пакета, и он не экспортируется. Например, `usecase/trip/create` описывает
`tripRepository` (`Save`, `GetByID`), `idempotencyStore` (`Find`, `Reserve`,
`Complete`), `txManager` (`Do`) и `tripMetrics` (`TripCreated`). Одна и та же
структура `repository/trip.Repository` подходит и под `tripRepository`, и под
`idempotencyStore`. Ручки объявляют свой `useCase`, роутер свои `createHandler`
и `tripHandler`, `health` объявляет `pinger`.

Так сделано по трём причинам. Потребитель сам формулирует, что ему нужно, и
не привязан к конкретной БД. В тестах интерфейс заменяется моком `gomock`,
БД не нужна. Когда у сервиса появится второй транспорт, он подключится к тем
же `usecase`, а менять `usecase` и репозиторий не придётся. Интерфейсы узкие:
самый большой содержит три метода.

Моки генерируются директивой `//go:generate go tool mockgen ...` в
`contract.go`, лежат рядом в `mocks_test.go` и закоммичены.

### Сборка зависимостей

`main` читает конфиг, вызывает `container.New(ctx, cfg)` и запускает то, что
тот вернул. Всё остальное делает `container`: поднимает телеметрию, пул,
репозиторий, `usecase`, ручки и собирает роутер. Глобальных переменных и
`slog.Default()` в коде нет, логгер и провайдеры телеметрии передаются через
конструкторы.

### Ошибки

Домен возвращает `ErrTripNotFound`, `ErrTripCompleted`, `ErrDriverBusy`,
`ErrIdempotencyConflict` и `*ValidationError`. Остальные слои оборачивают их
через `%w`. Функция `domainerror.Problem(err)` определяет статус и код по
`errors.Is` и `errors.As`, ручки просто передают ей ошибку. Неизвестная
ошибка становится `500 internal_error` без деталей, причина уходит в лог.

### Отступления от примерной раскладки

- `api/` и `migrations/` лежат в корне. Код из OpenAPI генерируется в `api/`
  с первой работы, переносить его ради раскладки не стали.
- Ключи идемпотентности хранит `repository/trip` рядом с поездками, отдельного
  пакета нет. `usecase` видит их через узкий интерфейс `idempotencyStore`,
  отдельный пакет ничего бы не добавил.
- Добавлены пакеты `http/router` и `http/health`. Роутер собирает ручки и
  определяет маршрут для middleware, а `health` не относится ни к одной ручке
  поездки.

## Наблюдаемость

Трейсы, метрики и логи уходят в коллектор по OTLP/HTTP, адрес берётся из
`OTEL_EXPORTER_OTLP_ENDPOINT`. Коллектор отдаёт трейсы в Jaeger и метрики в
Prometheus, Grafana читает Prometheus.

Если коллектор недоступен, сервис всё равно запускается и отвечает на запросы.
SDK раз в несколько секунд пишет в stderr ошибку отправки, на работу ручек это
не влияет.

### Трейсы

Middleware `Tracing` создаёт на каждый запрос корневой спан с именем
`{method} {route}`, например `POST /api/v1/trips`. В имени стоит шаблон
маршрута, а не URL. На ответах 5xx и при панике спан получает статус
`codes.Error` и атрибут `error.type`. Заголовок `traceparent` во входящем
запросе продолжает чужой трейс.

Каждый запрос к БД становится вложенным спаном. Это делает `otelpgx` на
уровне пула соединений. Небольшая обёртка в `infra/postgres/tracer.go`
дописывает в спан `db.operation.name` и `db.collection.name` (имя таблицы
берётся из текста SQL) и `error.type` при ошибке. В спан попадает текст
запроса с плейсхолдерами, значений параметров там нет.

### Метрики

| Имя | Тип | Единица | Лейблы |
|---|---|---|---|
| `http.server.request.duration` | histogram | секунды | `http.request.method`, `http.route`, `http.response.status_code` |
| `http.server.active_requests` | up-down counter | штуки | `http.request.method`, `http.route` |
| `db.client.operation.duration` | histogram | секунды | `db.operation.name`, `db.collection.name`, `error.type` |
| `tripgo.trips.created` | counter | штуки | нет |
| `tripgo.trips.completed` | counter | штуки | нет |

В `http.route` всегда шаблон маршрута, без подставленного `tripId`. Счётчики
`tripgo.trips.*` увеличиваются в `usecase` после успешного коммита. В
Prometheus метрики видны как `http_server_request_duration_seconds` и
`tripgo_trips_created_total`.

### Логи

Логи пишутся через `log/slog` в JSON в stdout. Обёртка над handler достаёт
из `context` идентификаторы спана и добавляет `trace_id` и `span_id`, поэтому
всё, что залогировано с `InfoContext(ctx, ...)` внутри запроса, связано с его
трейсом. Поле `service` задано у логгера один раз.

Middleware `Logging` пишет одну запись `request handled` на запрос с полями
`method`, `route`, `status` и `duration_ms`. Уровень `Info` для ответов 2xx и
4xx, `Error` только для 5xx. Тела запросов и ответов не логируются. События
«поездка создана» и «поездка завершена» пишет `usecase`, а не middleware.

### Middleware

Порядок такой: `Recover`, `Tracing`, `Logging`, `Metrics`, затем роутер.
`Recover` перехватывает панику, пишет стектрейс в лог и отвечает
`500 internal_error`. Остальные три middleware при панике учитывают запрос как
ответ 500, поэтому в трейсе, логе и метриках он не теряется.

Шаблон маршрута определяет сам роутер: до запуска цепочки он ищет подходящий
маршрут в chi и кладёт результат в `context`. Это нужно, потому что маршрут
должен быть известен уже в начале запроса: от него зависят имя спана и лейблы
`http.server.active_requests`. Неизвестному пути достаётся маршрут
`unmatched`, чтобы не создавать новую серию метрик на каждый адрес.

## Тесты

`make test` запускает unit-тесты с `-race -count=2` и показывает покрытие.

- `domain`: табличные тесты завершения поездки и валидации.
- `usecase`: табличные тесты на моках `gomock`, без БД. Покрыты создание,
  занятый водитель, невалидные данные, ошибка репозитория, завершение,
  повторное завершение, поездка не найдена и четыре случая идемпотентности:
  новый ключ, повтор с тем же телом, тот же ключ с другим телом и проигранная
  гонка за ключ.
- `http`: хендлеры через `httptest` с настоящим `domainerror`, отдельные
  таблицы для маппинга ошибок и для middleware (спаны проверяются через
  `tracetest`, метрики через `ManualReader`).

`make test-integration` запускает те же тесты и вместе с ними интеграционные
тесты репозитория на PostgreSQL (файлы с тегом `integration`). Нужны
загруженный `.env` и применённые миграции. Каждый тест создаёт поездки со
случайными UUID и удаляет их за собой, поэтому тесты можно запускать
повторно. Среди них тест отката транзакции: оба `INSERT` (в `trips` и в
`trip_status_history`) откатываются вместе, и тест параллельного `finish`.

`make lint` запускает `golangci-lint` с конфигом курса.

## Docker

```bash
docker build -t trip-service .
docker run --rm -p 8080:8080 --env-file .env trip-service
```

Сборка двухэтапная: `golang:1.27-alpine` компилирует статический бинарник,
в финальном образе на `alpine:3.20` лежит только он, без Go-тулчейна и
исходников, процесс запускается от `nobody`. Если PostgreSQL поднят на
хост-машине (как при локальной разработке через `tripgoctl`), значение
`localhost` в `DATABASE_URL` внутри контейнера указывает на сам контейнер, а
не на хост: замените его на `host.docker.internal`.

## Решения

### Уровень изоляции транзакций

`Read Committed`. Запрет двух активных поездок у одного водителя обеспечивает
partial unique index на уровне БД, и это не зависит от уровня изоляции. При
создании поездки внутри транзакции идут только независимые `INSERT`. В
`finish` есть цепочка «прочитал, решил, записал», но её защищает
`SELECT ... FOR UPDATE`: параллельный запрос ждёт снятия блокировки, потом
видит уже завершённую поездку и получает `trip_completed`. `Serializable`
добавил бы только повторы транзакций при конфликтах, а то, что нужно
сервису, уже даёт связка `Read Committed`, индекса и блокировки строки.

### Менеджер транзакций

Пакет `internal/infra/postgres/transaction`, тип `Manager` с методом
`Do(ctx, fn)`. Интерфейс `txManager` объявляют сами `usecase`, которым нужны
транзакции. Транзакция помещается в `context.Context` через неэкспортируемый
ключ, чтобы её нельзя было случайно перепутать с чужим значением в
контексте. Репозиторий получает исполнителя запроса (`*pgxpool.Pool` или
`pgx.Tx`) через метод `Manager.Executor(ctx)`: он проверяет контекст, и если
транзакция там уже есть, использует её, иначе обращается напрямую к пулу
соединений. Если `fn` вернула `nil`, выполняется `COMMIT`; если вернула ошибку
или запаниковала, выполняется `ROLLBACK`, при панике через `defer`/`recover` с
повторным `panic` после отката, чтобы вызывающий код узнал о ней. Вложенный
вызов `Do` определяется по наличию транзакции в переданном `ctx`: если она
уже есть, `Do` не открывает новую, а выполняет `fn` в существующей.

### Запрет двух активных поездок на одного водителя

Обеспечивает partial unique index:

```sql
CREATE UNIQUE INDEX trips_one_active_per_driver_idx
    ON trips (driver_id)
    WHERE status = 'active';
```

Уникальность требуется только среди строк со `status = 'active'`; завершённых
поездок у водителя может быть сколько угодно. Postgres проверяет это условие
атомарно при `INSERT`, поэтому два параллельных запроса на одного водителя
гарантированно дают одну созданную поездку и одну ошибку `23505`. Репозиторий
превращает её в `ErrDriverBusy` (и дальше в `409 driver_busy`), только если
нарушен именно этот индекс: другое нарушение уникальности стало бы `500`, а
не ложным «водитель занят».

### Идемпотентность создания поездки

Ключи лежат в таблице `idempotency_keys` (`key UUID PRIMARY KEY`,
`request_hash`, `response_body`). Логика живёт в `usecase/trip/create`, хэш
тела считает ручка.

Если в запросе есть `Idempotency-Key`, `usecase` сначала ищет ключ. Если
запись есть и хэш совпал, он возвращает ту же поездку с кодом `200`; если хэш
другой, возвращает `ErrIdempotencyConflict`. Поездка при повторе читается из
БД по `trip_id`, который лежит в `response_body` как `{"trip_id": "..."}`.
Поэтому, если поездку успели завершить, повторный запрос покажет актуальный
статус.

Если записи нет, в одной транзакции выполняются три шага: `Reserve` (вставка
ключа с хэшем тела), `Save` поездки и `Complete` (запись `trip_id`). Если
`Reserve` ничего не вставил, значит параллельный запрос с тем же ключом уже
занял его. Тогда транзакция откатывается целиком, вместе с созданной в ней
поездкой, `usecase` заново читает ключ и отвечает как на обычный повтор.

`Reserve` использует `INSERT ... ON CONFLICT DO NOTHING`, а не перехват
ошибки `23505`. В Postgres ошибка внутри транзакции переводит её в
прерванное состояние, а так конфликт просто даёт ноль вставленных строк, и
транзакция остаётся рабочей. Если конкурирующая транзакция ещё не
завершилась, вставка ждёт её конца. Гонка решается уникальным индексом на
уровне БД, а не блокировками в коде.

Время `started_at` и `finished_at` округляется до микросекунд перед записью:
Postgres хранит именно их, и ответ на создание совпадает с ответом на повтор
запроса до последней цифры.
