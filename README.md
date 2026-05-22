<div align="center">

  <img src="https://raw.githubusercontent.com/DANG-PH/DANG-PH/main/go-trans.png" alt="Go Gopher" width="100"/>

  <h1>Golang Game Service</h1>

  <p><em>High-performance real-time game server · Raw WebSocket · Protobuf · NATS · Redis</em></p>

  <p>
    <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=flat&logo=go&logoColor=white" alt="Go Version"/></a>
    <img src="https://img.shields.io/badge/WebSocket-gorilla-4A90D9?style=flat&logo=socketdotio&logoColor=white" alt="WebSocket"/>
    <img src="https://img.shields.io/badge/Protobuf-serialization-EA4335?style=flat&logo=protobuf&logoColor=white" alt="Protobuf"/>
    <img src="https://img.shields.io/badge/NATS-messaging-27AAE1?style=flat&logo=natsdotio&logoColor=white" alt="NATS"/>
    <img src="https://img.shields.io/badge/Redis-go--redis-DC382D?style=flat&logo=redis&logoColor=white" alt="Redis"/>
    <img src="https://img.shields.io/badge/Prometheus-metrics-E6522C?style=flat&logo=prometheus&logoColor=white" alt="Prometheus"/>
    <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-green.svg" alt="License"/></a>
  </p>

  <p>
    <a href="https://ws-go.dangpham.id.vn">
      <img src="https://img.shields.io/badge/▶_CHƠI_NGAY-ngocrongdark.com-FF6B35?style=for-the-badge&logoColor=white" alt="Play Now"/>
    </a>
  </p>
</div>

---

## Giới thiệu

Game server **real-time** viết bằng Go cho dự án **Ngọc Rồng Online** – tựa game MMORPG lấy cảm hứng từ *Dragon Ball (7 Viên Ngọc Rồng)*. Service này **chạy song song** với game service NestJS, đảm nhận riêng các luồng WebSocket cần **throughput cao và độ trễ cực thấp** — đồng bộ trạng thái (sync), tick rate, cập nhật vị trí theo FPS. Client kết nối đồng thời cả hai: NestJS cho nghiệp vụ, Go cho hot path real-time.

Service được thiết kế cho tick rate cao, dùng raw WebSocket cho kết nối client và Protobuf để serialize message gọn nhẹ. Nhận traffic qua domain `ws-go.dangpham.id.vn`, đứng sau Nginx load balancer và chạy trên 2 VPS ứng dụng (cổng `3010`).

> ⚠️ *Một số chi tiết triển khai dưới đây (biến `.env`, cách build) là phỏng đoán từ `go.mod` / cấu hình hạ tầng — bạn rà lại và sửa cho khớp implementation thực tế.*

---

## Vì sao tách Go riêng?

Hệ thống chia luồng WebSocket theo **tính chất công việc**, mỗi runtime lo phần nó mạnh nhất:

| | **NestJS** (`game_service`) | **Go** (`game-service-go`) — repo này |
|---|---|---|
| **Lo phần** | Business core | Hot path real-time |
| **Sự kiện** | Trade, buy item, nghiệp vụ giao dịch | Sync trạng thái, tick rate, cập nhật vị trí (FPS) |
| **Ưu tiên** | Tính đúng đắn, transactional | Throughput cao, độ trễ cực thấp |
| **Domain** | `ws.dangpham.id.vn` | `ws-go.dangpham.id.vn` |

Client mở **cả hai kết nối song song**: gửi sự kiện nghiệp vụ qua NestJS, còn các luồng tần suất cao (di chuyển, đồng bộ frame) đi qua Go — nơi mô hình goroutine xử lý hàng nghìn kết nối đồng thời với chi phí thấp.

---

## Tính năng chính

- **Raw WebSocket** (`gorilla/websocket`) — kết nối hai chiều, độ trễ thấp cho gameplay real-time.
- **Protobuf** — serialize message dạng nhị phân nhỏ gọn, thay cho custom binary protocol trước đây; schema rõ ràng, versioning dễ hơn, giảm bandwidth.
- **NATS** — messaging giữa các service / phân phối sự kiện độ trễ thấp.
- **Redis** (`go-redis/v9`) — lưu state, session, hoặc pub/sub đồng bộ giữa các instance.
- **JWT auth** (`golang-jwt/v5`) — xác thực kết nối WebSocket.
- **Prometheus metrics** — expose ở cổng `2112` cho monitoring (job `golang-game`).

---

## Stack & phụ thuộc

| Thành phần | Thư viện | Vai trò |
|---|---|---|
| WebSocket | `gorilla/websocket` | Kết nối client real-time |
| Serialization | `google.golang.org/protobuf` | Encode/decode message |
| Messaging | `nats-io/nats.go` | Pub/sub, giao tiếp service |
| Cache / state | `redis/go-redis/v9` | Lưu trạng thái, session |
| Auth | `golang-jwt/jwt/v5` | Xác thực JWT |
| ID | `google/uuid` | Sinh ID duy nhất |
| Config | `joho/godotenv` | Load biến môi trường từ `.env` |
| Metrics | `prometheus/client_golang` | Expose metrics |

---

## Vị trí trong hệ thống

```
                          Client (game)
                    ┌───────────┴───────────┐
            ws (nghiệp vụ)            ws-go (real-time)
                    │                       │
        ws.dangpham.id.vn          ws-go.dangpham.id.vn
                    │                       │
              ┌─────┴─────┐          ┌──────┴──────┐
              ▼           ▼          ▼             ▼
        NestJS game (3009)      game-service-go (3010)  ← repo này
        (trade, buy item)       (sync, tick, FPS)
                                       │
                          Nginx LB: least_conn · keepalive
                          ┌────────────┴────────────┐
                          ▼                          ▼
                  103.116.52.222:3010       103.116.52.219:3010
                          │                          │
              ┌───────────┼────────────┬─────────────┘
              ▼           ▼            ▼
            Redis        NATS      (DB / service khác)
                          │
                   metrics :2112 ──> Prometheus ──> Grafana
```

Hạ tầng (Nginx, Redis, NATS, Prometheus, các database) nằm ở repo [**dragonboy-nginx-service**](https://github.com/DANG-PH/dragonboy-nginx-service).

---

## Bắt đầu

**Yêu cầu:** Go 1.25+, một Redis và NATS đang chạy (có thể dùng stack trong repo hạ tầng).

```bash
# Clone
git clone https://github.com/DANG-PH/game-service-go.git
cd game-service-go

# Cấu hình
cp .env.example .env     # điền REDIS, NATS, JWT secret...

# Cài dependency
go mod download

# Chạy
go run .
```

> *Cách build/run trên đây là chuẩn Go — nếu repo có Dockerfile hoặc Makefile thì thay bằng lệnh tương ứng.*

---

## Cấu hình (`.env`)

| Biến | Ý nghĩa |
|---|---|
| `PORT` | Cổng WebSocket (mặc định `3010`) |
| `METRICS_PORT` | Cổng Prometheus metrics (`2112`) |
| `REDIS_ADDR` / `REDIS_PASSWORD` | Kết nối Redis |
| `NATS_URL` | Kết nối NATS |
| `JWT_SECRET` | Khoá ký/verify JWT |

> *Bảng trên là biến điển hình — chỉnh theo `.env.example` thực tế của repo.*

---

## Monitoring

Service expose metrics theo chuẩn Prometheus ở `:2112/metrics`. Prometheus (trong repo hạ tầng) scrape từ cả 2 instance:

```yaml
- job_name: 'golang-game'
  metrics_path: '/metrics'
  static_configs:
    - targets: ['103.116.52.222:2112', '103.116.52.219:2112']
```

Dữ liệu hiển thị qua Grafana tại `grafana.ngocrongdark.com`.

---

## Liên quan

| Repo | Vai trò |
|---|---|
| [NgocRongOnline](https://github.com/DANG-PH/NgocRongOnline) | Game client (Java / LibGDX) |
| [dragonboy-nginx-service](https://github.com/DANG-PH/dragonboy-nginx-service) | Hạ tầng: Nginx, database, monitoring, backup |

---

<div align="center">
  <sub>Real-time game server · Ngọc Rồng Online · <a href="https://ngocrongdark.com">ngocrongdark.com</a></sub>
</div>