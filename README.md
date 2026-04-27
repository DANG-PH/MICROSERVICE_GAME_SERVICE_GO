<div align="center">

  <img src="https://raw.githubusercontent.com/DANG-PH/MICROSERVICE_GAME_SERVICE_GO/master/golang.png" alt="Go Gopher" width="100"/>

  <h1>Golang Base</h1>

  <p>Go project core tối giản, production-ready — skeleton mà mọi service đều bắt đầu từ đây.</p>

  <p>
    <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go" alt="Go Version"/></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-green.svg" alt="License"/></a>
    <a href="https://github.com/DANG-PH/golang-base/stargazers"><img src="https://img.shields.io/github/stars/DANG-PH/golang-base?style=flat&color=yellow" alt="Stars"/></a>
    <a href="CONTRIBUTING.md"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen.svg" alt="PRs Welcome"/></a>
    <a href="https://goreportcard.com/report/github.com/DANG-PH/golang-base"><img src="https://goreportcard.com/badge/github.com/DANG-PH/golang-base?v=2" alt="Go Report Card"/></a>
  </p>

  <p>
    <a href="#-triết-lý">Triết lý</a> •
    <a href="#-cấu-trúc">Cấu trúc</a> •
    <a href="#-có-sẵn-trong-repo">Có sẵn</a> •
    <a href="#-quy-tắc-mở-rộng">Quy tắc mở rộng</a> •
    <a href="#-bắt-đầu">Bắt đầu</a> •
    <a href="#%EF%B8%8F-makefile-commands">Makefile</a> •
    <a href="#-đóng-góp">Đóng góp</a>
  </p>

</div>

---

## 💡 Triết lý

Đây là **core tối thiểu** dùng chung cho mọi Go service — không hơn, không kém.

Không bao gồm web framework, ORM, hay bất kỳ business logic nào. Những thứ đó được thêm vào theo từng service dựa trên yêu cầu thực tế. Mục tiêu là một điểm khởi đầu sạch, có định hướng rõ ràng về structure mà không áp đặt lựa chọn tech stack.

> Clone về. Thêm dependencies. Ship service.

---

## 📁 Cấu trúc

```
golang-base/
│
├── cmd/
│   └── api/
│       └── main.go                    # Entry point — wire mọi thứ lại với nhau
│
├── internal/
│   ├── config/
│   │   └── config.go                  # Load và validate env config
│   │
│   ├── app/
│   │   └── app.go                     # Bootstrap — khởi tạo và chạy app
│   │
│   ├── transport/                     # Inbound — thế giới gọi vào chúng ta
│   │   ├── http/
│   │   │   ├── server.go              # HTTP server lifecycle, timeout, graceful shutdown
│   │   │   ├── router.go              # Đăng ký routes
│   │   │   └── middleware/
│   │   │       ├── logger.go          # Log method, path, status, latency mỗi request
│   │   │       └── recovery.go        # Bắt panic, trả 500 thay vì crash server
│   │   │
│   │   ├── grpc/                      # gRPC server — nhận RPC từ service khác gọi vào
│   │   │   ├── server.go              # Setup gRPC server, TLS, interceptor chain
│   │   │   └── interceptor/
│   │   │       └── logger.go          # Log interceptor
│   │   │
│   │   └── consumer/                  # Nhận message từ broker (RabbitMQ, Kafka, NATS)
│   │       └── handler.go             # Deserialize message, delegate tới service
│   │
│   ├── external/                      # Outbound — chúng ta gọi ra ngoài
│   │   ├── client/                    # Sync calls — gọi và chờ response
│   │   │   └── example.go             # gRPC/HTTP client → external service
│   │   └── messaging/                 # Async calls — publish và không chờ
│   │       ├── publisher.go           # Gửi message lên queue/topic
│   │       └── messages.go            # Định nghĩa message types
│   │
│   ├── shared/                        # Thêm khi có thứ dùng chung nhiều domain
│   │   ├── enums.go                   # OrderStatus, PaymentStatus, Role...
│   │   ├── types.go                   # UserID, Money, Timestamp...
│   │   └── errors.go                  # ErrNotFound, ErrUnauthorized...
│   │
│   └── user/                          # Domain ví dụ — thay bằng domain thật của bạn
│       ├── handler.go                 # HTTP layer: bind, validate, delegate
│       ├── service.go                 # Interface — những gì domain này expose ra ngoài
│       ├── service_impl.go            # Implementation — business logic
│       ├── repository.go              # Interface — contract truy cập dữ liệu
│       ├── model.go                   # Domain structs & DTOs
│       └── postgres/
│           └── repository.go          # Implement repository với Postgres
│
├── pkg/                               # Shared packages có thể export (rỗng mặc định)
│
├── .github/
│   └── workflows/
│       └── ci.yml                     # CI pipeline — lint, test, build
│
├── .air.toml                          # Cấu hình Air hot-reload
├── Dockerfile                         # Multi-stage production build
├── .dockerignore
├── .env.example                       # Template env vars — commit lên Git, không có secret
├── .gitignore
├── .golangci.yml                      # Cấu hình linter
├── go.mod
├── Makefile
└── README.md
```

**Tại sao structure này?**

- **`cmd/api/`** — theo [official Go project layout](https://go.dev/doc/modules/layout). `main.go` chỉ làm 1 việc: parse config, build app, run.
- **`internal/config/`** — mọi service đều cần config. Tập trung ở đây tránh `os.Getenv` rải rác.
- **`internal/app/`** — bootstrap pattern thấy ở mọi production repo. Giữ `main.go` gọn và app dễ test.
- **`internal/transport/`** — tất cả protocol **nhận vào** ở một chỗ. HTTP server, gRPC server, MQ consumer đều là cách nhận input — thuộc về nhau.
- **`internal/external/`** — tất cả thứ **gọi ra ngoài** ở một chỗ. Đối xứng hoàn toàn với `transport/`. `client/` cho sync calls, `messaging/` cho async publish.
- **`internal/shared/`** — chỉ tạo khi thực sự có 2+ domain cần dùng chung. Không tạo sớm.
- **`internal/<domain>/`** — layout theo domain. Mỗi domain sở hữu toàn bộ vertical slice của nó.
- **`pkg/`** — code dùng lại được giữa nhiều service. Rỗng mặc định.

---

## ✅ Có sẵn trong repo

Những file dưới đây đã được implement sẵn — clone về là dùng được ngay.

### `internal/config/config.go`

Load config từ environment variables, có giá trị default cho từng field. Không dùng thư viện ngoài — chỉ `os.Getenv` thuần. Thêm field mới vào struct `Config` và `Load()` là xong.

```
APP_ENV   → cfg.App.Env   (default: development)
APP_PORT  → cfg.App.Port  (default: 8080)
```

### `internal/app/app.go`

Bootstrap toàn bộ app. Khởi tạo HTTP server với đúng timeout, đăng ký routes, xử lý graceful shutdown khi nhận `SIGINT`/`SIGTERM`. Server chờ request đang chạy hoàn thành (tối đa 10 giây) trước khi tắt.

Endpoint `/health` trả `{"status":"ok"}` đã có sẵn — dùng ngay cho liveness check.

### `internal/transport/http/server.go`

Tạo `*http.Server` với timeout production-ready:

| Timeout | Giá trị | Mục đích |
|---|---|---|
| `ReadTimeout` | 10s | Chặn slow client gửi request |
| `WriteTimeout` | 10s | Giới hạn thời gian ghi response |
| `IdleTimeout` | 60s | Đóng idle connection sau 60s |

### `internal/transport/http/middleware/logger.go`

Log mỗi request: `POST /v1/users 201 23ms`. Wrap `http.ResponseWriter` để capture status code sau khi handler chạy xong.

### `internal/transport/http/middleware/recovery.go`

Bắt `panic` trong bất kỳ handler nào, log stack trace đầy đủ, trả `500` thay vì crash server. **Bắt buộc phải có ở production** — thiếu nó thì 1 panic kill cả process.

### `Dockerfile`

Multi-stage build: stage 1 compile binary, stage 2 chạy trên `alpine:3.19` (~15MB image). Binary build với `-ldflags="-s -w"` để strip debug info, giảm size.

---

## 🏛️ Domain Architecture

Mỗi domain theo pattern layered với interface/implementation tách biệt:

```
Request đến
  └── handler.go          ← HTTP only: bind JSON, validate, delegate
        └── service.go        ← Interface: contract của domain
        └── service_impl.go   ← Implementation: business rules
              └── repository.go       ← Interface: data access contract
              └── postgres/
                    └── repository.go ← Implementation: SQL queries
                          └── model.go ← Structs, DTOs
```

**Quy tắc bất biến:**
- Handler không bao giờ truy cập database trực tiếp.
- Service không biết về HTTP — không có `http.Request`, không có status code.
- Repository không chứa business rule.
- `service.go` định nghĩa interface; `service_impl.go` implement — mock khi test cực dễ.
- `repository.go` định nghĩa interface; `postgres/repository.go` implement — đổi DB chỉ cần thêm subfolder mới.
- Model chia sẻ tự do trong domain, không chia sẻ giữa các domain.

---

## 🔌 Quy tắc mở rộng

> Đọc phần này trước khi thêm bất kỳ file hay folder mới nào vào project.

---

### 1. Thêm domain mới

Mỗi tính năng nghiệp vụ = 1 folder trong `internal/`.

```
internal/
└── order/                  # tên domain, số ít, chữ thường
    ├── handler.go           # nhận HTTP request, bind, validate, gọi service
    ├── service.go           # interface OrderService { ... }
    ├── service_impl.go      # struct orderService implement interface trên
    ├── repository.go        # interface OrderRepository { ... }
    ├── model.go             # Order struct, CreateOrderRequest, OrderResponse
    └── postgres/
        └── repository.go   # implement OrderRepository với GORM/sqlx
```

**Quy tắc đặt tên file:**

| File | Nội dung | Quy tắc |
|---|---|---|
| `handler.go` | HTTP handlers | 1 file nếu ít route, tách `handler_admin.go` nếu nhiều |
| `service.go` | Interface | Chỉ chứa interface, không chứa logic |
| `service_impl.go` | Implementation | Struct + methods implement interface |
| `repository.go` | Interface | Chỉ chứa interface |
| `model.go` | Structs | Domain entity + request/response DTOs cùng 1 file |
| `postgres/repository.go` | DB impl | Nếu dùng MySQL thì tạo `mysql/repository.go` |

**Không được:**
```
❌ internal/order/orderHandler.go   — không dùng camelCase cho tên file
❌ internal/order/order_handler.go  — không prefix tên domain vào file
❌ internal/order/handlers/         — không tạo subfolder cho từng layer
```

---

### 2. Thêm transport mới

Transport = cách nhận input từ bên ngoài. Tất cả nằm trong `internal/transport/`.

**Thêm WebSocket:**
```
internal/transport/
└── ws/
    ├── server.go       # upgrade HTTP → WS, quản lý hub
    ├── hub.go          # quản lý connections, broadcast
    └── client.go       # 1 goroutine/client: readPump + writePump
```

**Thêm gRPC server:**
```
internal/transport/
└── grpc/
    ├── server.go
    └── interceptor/
        ├── auth.go     # metadata token check
        └── logger.go
```

**Thêm consumer (MQ):**
```
internal/transport/
└── consumer/
    ├── handler.go          # entry point — subscribe và dispatch
    ├── order_handler.go    # xử lý order events
    └── user_handler.go     # xử lý user events
```

**Không được:**
```
❌ internal/websocket/      — phải nằm trong transport/
❌ internal/transport/websocketServer.go — phải là folder, không phải file đơn lẻ
```

---

### 3. Thêm outbound call

Mọi thứ gọi ra ngoài nằm trong `internal/external/`.

**Gọi service khác (sync):**
```
internal/external/
└── client/
    ├── payment.go      # gRPC/HTTP client → payment service
    └── inventory.go    # gRPC/HTTP client → inventory service
```

Mỗi file client:
- Định nghĩa interface ở đầu file (để `service_impl.go` inject)
- Implement gRPC stub bên dưới
- Không chứa business logic, chỉ wrap network call

```go
// internal/external/client/payment.go

type PaymentClient interface {
    Charge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error)
}

type grpcPaymentClient struct { conn *grpc.ClientConn }

func NewPaymentClient(addr string) (PaymentClient, error) { ... }
func (c *grpcPaymentClient) Charge(...) { ... }
```

**Publish message (async):**
```
internal/external/
└── messaging/
    ├── publisher.go    # implement publish lên broker
    └── messages.go     # định nghĩa tất cả message types
```

```go
// internal/external/messaging/messages.go
type OrderCreatedEvent struct {
    OrderID   string
    UserID    string
    CreatedAt time.Time
}
```

**Không được:**
```
❌ internal/payment/client.go   — client phải nằm trong external/client/
❌ internal/rabbitmq/           — infra chi tiết không expose ra tên folder
```

---

### 4. Thêm middleware HTTP

Tất cả middleware nằm trong `internal/transport/http/middleware/`.

```
middleware/
├── logger.go       # đã có sẵn
├── recovery.go     # đã có sẵn
├── auth.go         # thêm khi cần JWT verify
├── ratelimit.go    # thêm khi cần rate limiting
├── cors.go         # thêm khi cần CORS
└── idempotency.go  # thêm khi cần idempotency key check
```

Mỗi middleware là 1 file riêng. Tên file = đúng tên chức năng. Đăng ký thứ tự trong `router.go`:

```go
// router.go — thứ tự middleware quan trọng
handler = middleware.Recovery(handler)   // ngoài cùng — bắt mọi panic
handler = middleware.Logger(handler)     // sau recovery
handler = middleware.CORS(handler)       // trước auth
handler = middleware.Auth(handler)       // trong cùng — chạy cuối trước handler
```

**Không được:**
```
❌ internal/middleware/         — phải nằm trong transport/http/middleware/
❌ middleware/authMiddleware.go  — không suffix "Middleware" vào tên file
```

---

### 5. Thêm interceptor gRPC

Tương tự middleware nhưng dùng tên `interceptor` — đúng với gRPC spec.

```
internal/transport/grpc/interceptor/
├── logger.go       # đã có sẵn
├── auth.go         # metadata token check
└── recovery.go     # bắt panic trong RPC handler
```

---

### 6. Thêm shared types

Chỉ tạo `internal/shared/` khi **2+ domain** cần dùng cùng 1 thứ.

```
internal/shared/
├── enums.go        # const + type cho status, role, loại...
├── types.go        # custom types: UserID, Money, Timestamp
└── errors.go       # sentinel errors dùng chung
```

```go
// internal/shared/errors.go
var (
    ErrNotFound     = errors.New("not found")
    ErrUnauthorized = errors.New("unauthorized")
    ErrConflict     = errors.New("conflict")
)
```

**Không được:**
```
❌ internal/shared/user_enums.go    — nếu chỉ user dùng thì để trong user/model.go
❌ internal/shared/utils.go         — "utils" là dump folder, không được dùng
❌ internal/common/                  — tương tự, không dùng tên generic
```

---

### 7. Thêm pkg utility

`pkg/` chỉ dành cho code **reusable giữa nhiều service**.

```
pkg/
├── response/
│   └── response.go     # JSON format chuẩn {success, data, error, meta}
├── apperror/
│   └── apperror.go     # custom error type với HTTP status code
├── logger/
│   └── logger.go       # zap/slog wrapper
├── paginate/
│   └── paginate.go     # offset + cursor pagination
├── telemetry/
│   └── telemetry.go    # OpenTelemetry setup — tracing + metrics
└── health/
    └── health.go       # /health + /ready handlers
```

Mỗi package trong `pkg/` là 1 folder riêng với 1 file chính cùng tên. Nếu lớn hơn thì tách file nhưng giữ cùng package name.

**Không được:**
```
❌ pkg/utils/           — không dùng tên generic
❌ pkg/helpers/         — tương tự
❌ pkg/user/            — domain logic không vào pkg/
❌ pkg/jwt/jwt.go rồi import internal/ — pkg không được import internal/
```

---

### 8. Thêm database migration

```
migrations/
└── postgres/
    ├── 001_create_users.sql
    ├── 002_create_orders.sql
    └── 003_add_index_orders_user_id.sql
```

**Quy tắc đặt tên:**
- Prefix số tăng dần: `001`, `002`, `003`...
- Tên mô tả rõ action: `create_<table>`, `add_<column>_<table>`, `add_index_<table>_<column>`
- Không đổi tên file sau khi đã commit — migration tool dựa vào tên file để track version

---

### 9. Thêm tests

**Unit test** — đặt cùng file source:
```
internal/user/
├── service_impl.go
└── service_impl_test.go    # test file cùng package
```

**Integration test** — cần infra thật (DB, Redis):
```
test/
└── integration/
    ├── user_test.go        # dùng testcontainers-go spin up DB thật
    └── order_test.go
```

**E2E test** — test toàn bộ HTTP flow:
```
test/
└── e2e/
    └── auth_test.go        # gửi HTTP request thật, check response
```

**Mock** — generate bằng mockery:
```
test/
└── mock/
    ├── user_service.go     # auto-generated mock của UserService interface
    └── user_repository.go  # auto-generated mock của UserRepository interface
```

---

### 10. Thêm binary mới

Khi cần thêm worker, CLI, hoặc service chạy riêng:

```
cmd/
├── api/
│   └── main.go         # đã có — HTTP API server
├── worker/
│   └── main.go         # thêm khi cần background job processor
└── migrate/
    └── main.go         # thêm khi cần CLI chạy migration
```

Mỗi binary trong `cmd/` chỉ làm 1 việc: load config → gọi `app.New()` → run. Không có business logic trong `main.go`.

---

### 11. Thêm proto definitions

```
api/
└── proto/
    ├── user/
    │   └── user.proto
    └── order/
        └── order.proto
```

Generated Go code từ proto **không commit** vào repo — generate lại trong CI bằng `make proto`. Nếu muốn commit thì tạo folder riêng:

```
internal/transport/grpc/pb/
├── user/
│   └── user.pb.go      # generated
└── order/
    └── order.pb.go     # generated
```

---

### 12. Thêm config mới

Không bao giờ dùng `os.Getenv` trực tiếp trong code — luôn thêm vào `internal/config/config.go`:

```go
// Thêm struct
type DatabaseConfig struct {
    Host     string
    Port     int
    Name     string
    User     string
    Password string
}

// Thêm vào Config
type Config struct {
    App      AppConfig
    Database DatabaseConfig   // thêm vào đây
}

// Thêm vào Load()
Database: DatabaseConfig{
    Host: getEnv("DB_HOST", "localhost"),
    Port: getEnvInt("DB_PORT", 5432),
    ...
}
```

Luôn thêm var mới vào `.env.example` cùng lúc với code.

---

### Tổng hợp: file mới đặt ở đâu

| Loại | Đặt ở |
|---|---|
| Feature nghiệp vụ mới | `internal/<domain>/` |
| HTTP handler | `internal/<domain>/handler.go` |
| Business logic | `internal/<domain>/service_impl.go` |
| DB query | `internal/<domain>/postgres/repository.go` |
| HTTP middleware | `internal/transport/http/middleware/<name>.go` |
| gRPC interceptor | `internal/transport/grpc/interceptor/<name>.go` |
| WebSocket | `internal/transport/ws/` |
| MQ consumer | `internal/transport/consumer/<name>_handler.go` |
| Gọi service khác (sync) | `internal/external/client/<service>.go` |
| Publish message (async) | `internal/external/messaging/` |
| Enums/types dùng chung | `internal/shared/enums.go` hoặc `types.go` |
| Sentinel errors dùng chung | `internal/shared/errors.go` |
| Utility reusable | `pkg/<name>/<name>.go` |
| SQL migration | `migrations/postgres/NNN_<action>_<table>.sql` |
| Unit test | Cùng folder với file test (`_test.go`) |
| Integration test | `test/integration/` |
| E2E test | `test/e2e/` |
| Proto definition | `api/proto/<domain>/<domain>.proto` |
| Binary mới | `cmd/<name>/main.go` |
| Config mới | `internal/config/config.go` + `.env.example` |

---

## 📦 `pkg/` — Khi nào dùng

`pkg/` cố tình để rỗng. Chỉ thêm khi đáp ứng **cả 2** tiêu chí:

1. Code **không** đặc thù cho business logic của service này.
2. Service khác có thể copy sang dùng mà không cần sửa.

**Phù hợp:**

```
pkg/
├── response/      # HTTP response format chuẩn
├── apperror/      # Custom error types với HTTP status mapping
├── logger/        # Structured logger wrapper
├── telemetry/     # OpenTelemetry tracing + Prometheus metrics
├── health/        # /health + /ready endpoints
├── paginate/      # Offset và cursor-based pagination
└── testutil/      # Test helpers
```

**Không nên:**

```
❌ pkg/jwt/        — không phải service nào cũng dùng JWT
❌ pkg/auth/       — auth logic là business logic
❌ pkg/database/   — config DB là quyết định của từng service
❌ pkg/utils/      — tên generic, không được phép
```

---

## 🚀 Bắt đầu

### Yêu cầu

Cài đặt các công cụ sau trước khi bắt đầu:

#### 1. Cài Go 1.22+

**Windows:**
- Tải installer tại [https://golang.org/dl/](https://golang.org/dl/) → chọn file `.msi`
- Chạy installer, mặc định cài vào `C:\Program Files\Go`
- Mở terminal mới, kiểm tra:
```powershell
go version
# → go version go1.22.x windows/amd64
```

**Linux/Mac:**
```bash
# Mac (Homebrew)
brew install go

# Ubuntu/Debian
sudo apt install golang-go

# Kiểm tra
go version
```

#### 2. Cài Make

**Windows** — Make không có sẵn, cài qua Chocolatey:
```powershell
# Cài Chocolatey trước (chạy PowerShell với quyền Admin)
Set-ExecutionPolicy Bypass -Scope Process -Force
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))

# Sau đó cài Make
choco install make

# Kiểm tra
make --version
```

**Linux/Mac:**
```bash
# Mac
brew install make

# Ubuntu/Debian
sudo apt install make

# Kiểm tra
make --version
```

#### 3. Cài Air (hot-reload)

```bash
go install github.com/air-verse/air@latest
```

Kiểm tra:
```bash
air -v
```

> **Windows:** Sau lần đầu chạy `make dev`, Windows Firewall sẽ hỏi cấp quyền cho `tmp/api.exe` — bấm **Allow** một lần duy nhất, không hỏi lại.

#### 4. Docker (tùy chọn — chỉ cần khi dùng `make docker/*`)

Tải tại [https://docs.docker.com/get-docker/](https://docs.docker.com/get-docker/)

---

### Setup Linux/Mac

```bash
# 1. Clone
git clone https://github.com/DANG-PH/golang-base.git my-service
cd my-service

# 2. Đổi import path trong code (.go)
# (dùng full module path, không dùng "my-service")
find . -type f -name "*.go" -exec sed -i.bak 's|github.com/DANG-PH/golang-base|github.com/DANG-PH/my-service|g' {} +
find . -type f -name "*.bak" -delete

# 3. Đổi module chuẩn bằng Go toolchain (KHÔNG edit tay go.mod)
go mod edit -module github.com/DANG-PH/my-service
go mod tidy

# 4. Reset git history
rm -rf .git && git init
git add .
git commit -m "chore: initial from golang-base"

# 5. Cấu hình env
cp .env.example .env

# 6. Chạy với hot-reload
make dev

# 7. Test
curl http://localhost:8080/health
# → {"status":"ok"}
```

### Setup Windows (PowerShell)

```powershell
# 1. Clone
git clone https://github.com/DANG-PH/golang-base.git my-service
cd my-service

# 2. Replace import path trong code (.go)
Get-ChildItem -Recurse -Filter "*.go" | % { [System.IO.File]::WriteAllText($_.FullName, ([System.IO.File]::ReadAllText($_.FullName) -replace 'github.com/DANG-PH/golang-base','github.com/DANG-PH/my-service'), (New-Object System.Text.UTF8Encoding($false))) }

# 3. Update module đúng chuẩn (tránh BOM + lỗi syntax)
go mod edit -module github.com/DANG-PH/my-service
go mod tidy

# 4. Reset git history
Remove-Item -Recurse -Force .git; git init
git add .
git commit -m "chore: initial from golang-base"

# 5. Cấu hình env
Copy-Item .env.example .env

# 6. Chạy với hot-reload
make dev

# 7. Test
curl http://localhost:8080/health
# → {"status":"ok"}
```

---

### Cách `make dev` hoạt động

```
Bạn save file .go
       ↓
Air detect thay đổi (file watcher)
       ↓
Auto rebuild → tmp/api.exe
       ↓
Kill process cũ → Start process mới
       ↓
Server đã chạy code mới — không cần làm gì
```

Terminal sẽ hiện:
```
building...
running...
```

Không cần `Ctrl+C` rồi chạy lại tay. Chỉ việc code, save, test.

---

## ⚙️ Cấu hình

```env
APP_ENV=development   # development | staging | production
APP_PORT=8080
GRPC_PORT=9090        # thêm khi dùng gRPC
```

Thêm vars mới vào `.env.example` khi service lớn dần — không bao giờ commit `.env`.

---

## 🛠️ Makefile Commands

| Command | Mô tả |
|---|---|
| `make dev` | **Chạy với hot-reload (Air) ← dùng hàng ngày** |
| `make run` | Chạy application (fallback nếu không có Air) |
| `make build` | Compile ra `./bin/api` |
| `make test` | Chạy tất cả tests với race detector |
| `make test/cover` | Hiển thị coverage report |
| `make lint` | Chạy `golangci-lint` |
| `make fmt` | Chạy `go fmt` |
| `make vet` | Chạy `go vet` |
| `make tidy` | Chạy `go mod tidy` |
| `make clean` | Xóa build artifacts |
| `make docker/build` | Build Docker image |
| `make docker/run` | Chạy Docker container với `.env` |
| `make proto` | Compile `.proto` files → Go code |

---

## 🔄 CI Pipeline

GitHub Actions chạy trên mọi push vào `main` và mọi pull request:

- **lint** — `golangci-lint` với `.golangci.yml`
- **test** — `go test -race ./...` với coverage report

---

## 🤝 Đóng góp

1. Fork repository
2. Tạo branch (`git checkout -b feature/your-feature`)
3. Commit theo [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `chore:`)
4. Push và mở Pull Request

PR thêm code framework-specific hoặc business-specific sẽ không được merge — repo này giữ generic.

---

## 👤 Tác giả

<p align="left">
  <a href="https://github.com/DANG-PH">
    <img src="https://github.com/DANG-PH.png" width="72" style="border-radius:50%" alt="DANG-PH"/>
  </a>
  <br/>
  <strong>Phạm Hải Đăng</strong> &nbsp;·&nbsp; <a href="https://github.com/DANG-PH">@DANG-PH</a>
  <br/>
  Backend Engineer · Go · NestJS · Distributed Systems
</p>

Nếu cái này tiết kiệm cho bạn 30 phút setup — để lại một ⭐. Nó giúp người khác tìm thấy repo này khi search Go starter.

---

## 📄 License

MIT — xem [LICENSE](LICENSE) để biết chi tiết.

<p align="center"><sub>Built with Go · Kept simple on purpose</sub></p>