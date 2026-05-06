# Giải thích `main.go` — Game Service Go

---

## Package Declaration

`package main` — khai báo **bắt buộc** ở đầu mọi file Go.

Tên `main` có ý nghĩa **đặc biệt**: báo Go đây là chương trình chạy được (executable), không phải library. Mọi file khác trong cùng folder cũng phải là `package main`.

> **So với NestJS:** NestJS không có khái niệm package, chỉ có module. File `main.ts` là entry point ngầm định, không cần khai báo gì. Go thì rõ ràng hơn: `main` + hàm `main()` = chương trình chạy được.

---

## Import Block

Go gom mọi import vào trong ngoặc `()`. Convention chia 3 nhóm cách nhau dòng trống:
1. **Standard library** — có sẵn trong Go, không cần cài
2. **Third-party packages** — từ GitHub, cài qua `go get`
3. **Internal packages** — code của project mình

Tool `goimports` tự động sắp xếp và xóa import thừa.

### `context`

Cơ chế truyền tín hiệu **hủy bỏ** và **deadline** xuyên suốt call chain.

Tưởng tượng: 1 request HTTP đi qua 5 layer (handler → service → repo → DB → cache). Nếu user hủy request, bạn muốn **tất cả 5 layer dừng ngay** để khỏi lãng phí tài nguyên. Context truyền "lệnh dừng" xuống tất cả các tầng.

> NestJS **không có** khái niệm này built-in — gần nhất là `AbortController` của JS.

### `log/slog`

Structured Logging (từ Go 1.21+).

**Structured** = log dạng key-value (JSON), không phải string thường. Vì sao? Production có hàng trăm server, log gom về log aggregator (ELK/Loki/Datadog...) — JSON parse nhanh hơn regex parse text rất nhiều.

> Tương đương Pino/Winston bên Node, nhưng `slog` là **standard library** nên không cần cài.

### `os`

Cầu nối nói chuyện với **hệ điều hành** (Operating System). OS là phần mềm trung gian giữa code và phần cứng (CPU/RAM/disk/network).

Khi chạy app, OS cấp RAM, cho mở file, mở socket, đọc env vars, gửi signal khi muốn dừng app.

| Go | Node/NestJS |
|---|---|
| `os.Getenv("KEY")` | `process.env.KEY` |
| `os.Exit(1)` | `process.exit(1)` |
| `os.Stdout` | `process.stdout` |
| `os.Args` | `process.argv` |

Bản chất giống hệt, chỉ là Node wrap sẵn còn Go bắt import thủ công.

### `os/signal`

Bắt **signal** từ OS. Signal là cách OS "ra lệnh" cho process đang chạy. Ví dụ:
- Nhấn `Ctrl+C` trong terminal → terminal gửi `SIGINT` đến app
- Docker chạy `docker stop` → Docker gửi `SIGTERM` đến container
- K8s xóa pod → K8s gửi `SIGTERM`, đợi 30s rồi gửi `SIGKILL`

App có thể **bắt signal** để cleanup trước khi chết (graceful shutdown).

### `syscall`

System Calls — các lệnh **gọi thẳng vào kernel** của OS.

"Kernel" là phần lõi của OS — có quyền tối cao trên máy. Bình thường app chạy ở "user space" (giới hạn quyền), khi cần tương tác với phần cứng/file/network phải gọi syscall vào "kernel space".

Ở đây chỉ dùng để lấy hằng số định danh signal:
- `syscall.SIGINT` = số 2 = Ctrl+C
- `syscall.SIGTERM` = số 15 = lệnh terminate lịch sự

### `time`

Xử lý thời gian, duration, timezone, timer. Ở đây dùng để tạo timeout 10 giây.

> Tương đương `Date` + `setTimeout` của JS gộp lại.

### `github.com/joho/godotenv`

Load file `.env` vào env vars của process. Đọc file → set vào `os.Getenv` → code đọc bằng `os.Getenv("DATABASE_URL")`.

> Tương đương lib `dotenv` bên Node mà NestJS hay dùng (qua `@nestjs/config`).

### Internal packages (`internal/`)

`internal/` là **folder đặc biệt** trong Go: chỉ package cùng module mới import được. Giống "private" ở mức module — bảo vệ không cho project khác import nhầm.

> NestJS không có cơ chế này — mọi thứ export đều public.

- `internal/config` — load + validate cấu hình từ env vars
- `internal/app` — khởi tạo + chạy + tắt ứng dụng (HTTP server, DB pool...)

---

## Hàm `main()` — Entry Point

`func main()` là entry point của chương trình. Bắt buộc:
- Nằm trong `package main`
- Tên `main`, không nhận tham số, không return

Khi chạy `./my-app`, Go runtime gọi `main()` đầu tiên. Khi `main()` return → chương trình kết thúc.

> **So với NestJS:** tương đương `async function bootstrap() {...}` trong `main.ts`. Khác biệt: NestJS dùng async/await (event loop single-thread), Go dùng goroutine + channel (concurrency thật sự song song trên nhiều CPU core).

---

## Bước 1 — Load file `.env`

`godotenv.Load()` đọc file `.env` ở thư mục hiện tại, parse từng dòng `KEY=VALUE`, rồi set vào env vars của process.

**Vì sao `_ = godotenv.Load()`?**

Hàm `Load()` trả về error nếu file `.env` không tồn tại. Nhưng ở production (Docker/K8s) không dùng file `.env` — env vars được inject qua `docker run -e` hoặc K8s ConfigMap/Secret. Local dev mới có file `.env`. Vậy nếu thiếu file thì kệ, không sao.

**Blank identifier `_`** — ý: "tao biết hàm này có return value nhưng tao cố tình bỏ qua".

> Go có quy tắc **cực nghiêm**: mọi biến khai báo phải dùng, mọi return value phải xử lý hoặc bỏ qua tường minh. Khác hẳn JS/TS vốn cho phép ignore tự do.

---

## Bước 2 — Khởi tạo Logger

Tạo structured logger ghi log JSON ra `stdout`.

**`stdout` (Standard Output) là gì?**

Mỗi process khi chạy có 3 luồng (stream) mặc định OS cấp:

| Stream | Số | Dùng cho |
|---|---|---|
| `stdin` | 0 | nhận input (gõ phím vào terminal) |
| `stdout` | 1 | output bình thường (`console.log`, `fmt.Println`) |
| `stderr` | 2 | output lỗi/log (`console.error`) |

Tách `stdout`/`stderr` để có thể chuyển hướng riêng: `./my-app > out.log 2> err.log`

Trong K8s/Docker: container engine tự động hứng `stdout`/`stderr` của app rồi forward đến hệ thống log. Vì vậy production luôn log ra `stdout`, **không ghi vào file**.

**Log Aggregator là gì?**

"Aggregate" = gom lại, tổng hợp. Có 50 con server, mỗi con in log ra `stdout`. Agent ở mỗi server đọc `stdout` → gửi về 1 server trung tâm (aggregator) → bạn vào dashboard 1 chỗ là thấy log của tất cả.

Tools phổ biến: ELK, Loki+Grafana, Datadog, Splunk, CloudWatch. JSON log nhanh hơn text log nhiều lần khi parse.

**Dấu `&` (pointer):**

`&slog.HandlerOptions{...}` — dấu `&` lấy **địa chỉ** (pointer) của struct. Hàm `NewJSONHandler` nhận `*HandlerOptions` chứ không nhận `HandlerOptions`. Truyền pointer = truyền địa chỉ memory, không copy.

> Trong NestJS không cần nghĩ vì JS object mặc định by reference.

**Log level:**

`slog` có 4 level: `Debug < Info < Warn < Error`. Set `Info` nghĩa là chỉ log từ Info trở lên, bỏ Debug. Production tránh log Debug vì quá nhiều, tốn tiền log aggregator.

`slog.SetDefault(log)` — set logger này làm default global. Các package khác gọi `slog.Info(...)` sẽ tự động dùng logger này.

---

## Bước 3 — Load Config

`config.Load()` là hàm tự viết. Thường nó:
1. Đọc env vars (`PORT`, `DATABASE_URL`...) qua `os.Getenv`
2. Validate (port phải là số, DB URL phải đúng format...)
3. Trả về struct `Config` + `error`

**Cú pháp `:=` — Short Variable Declaration:**

```go
cfg, err := config.Load()
```

Tương đương khai báo `var cfg ConfigType; var err error` nhưng Go tự suy ra type từ vế phải.

**Multiple Return Values — đặc sản của Go:**

Hàm Go có thể trả về nhiều giá trị: `(Config, error)`. Đây là pattern xử lý lỗi chuẩn của Go — thay vì throw exception như NestJS, Go trả về `error` như giá trị thường. Ai gọi hàm phải tự check.

- ✅ Ưu điểm: tường minh, nhìn code biết ngay chỗ nào có thể lỗi
- ❌ Nhược điểm: code dài hơn vì nhiều `if err != nil`

**Pattern check error chuẩn của Go** — sẽ thấy hàng chục lần trong project:

```go
if err != nil {
    log.Error("config load failed", "err", err)
    os.Exit(1)
}
```

`err != nil` nghĩa là có lỗi (`nil` = null/None bên ngôn ngữ khác).

> **Cảnh báo:** `os.Exit` **không chạy defer**. Ở `main()` lúc này chưa có resource nào cần cleanup nên OK. Nhưng đừng bao giờ `os.Exit` ở giữa hàm có `defer` — sẽ leak resource.

---

## Bước 4 — Khởi tạo App

`app.New(cfg, log)` là constructor function tự viết.

> Tương đương `NestFactory.create(AppModule)` trong NestJS.

Bên trong thường:
1. Tạo HTTP server (Gin/Echo/chi/net.http...)
2. Connect DB (Postgres/MySQL...)
3. Connect Redis/Cache
4. Wire dependencies (DI thủ công vì Go không có `@Injectable`)
5. Đăng ký routes/middleware

> Quy ước Go: hàm tạo instance đặt tên `New...` hoặc `NewXxx`. Go **không có** class/constructor — chỉ có struct + hàm thường. Đây là khác biệt lớn so với NestJS (vốn rất OOP).

---

## Bước 5 — Chạy App trong Goroutine

**Vấn đề:** `a.Run()` khởi động HTTP server, lắng nghe forever (blocking). Nếu chạy thẳng `a.Run()` thì code sau nó không bao giờ được chạy → không có cơ chế graceful shutdown. Phải chạy trong goroutine để main thread tiếp tục đợi signal.

**Channel là gì?**

"Ống dẫn" thread-safe để các goroutine gửi/nhận data.

> Triết lý của Go: *"Don't communicate by sharing memory; share memory by communicating"* — đừng chia sẻ biến rồi đặt lock, hãy gửi data qua channel.

```go
errCh := make(chan error, 1)
```

- `chan error` — channel chứa giá trị kiểu `error`
- Số `1` — buffer size = 1 phần tử
- **Buffered** (>0): gửi không block đến khi đầy buffer
- **Unbuffered** (=0): gửi block đến khi có người nhận

Buffer 1 vì: `a.Run()` có thể return error một lần, muốn nó gửi xong luôn rồi goroutine kết thúc, không bị treo chờ.

**Goroutine là gì?**

```go
go func() {
    errCh <- a.Run()
}()
```

- Đơn vị thực thi siêu nhẹ của Go (~2KB stack ban đầu, thread OS thường 1-8MB)
- Có thể tạo **hàng triệu** goroutine cùng lúc
- Go scheduler tự phân phối goroutine lên các OS thread → chạy thật sự song song trên nhiều CPU core

| | Node/NestJS | Go |
|---|---|---|
| Model | Single-threaded event loop | Multi-threaded goroutines |
| `await fetch()` | Đăng ký callback, vẫn 1 thread | Goroutine thật sự song song |
| CPU core | Dùng 1 core | Dùng được tất cả core |

Toán tử channel:
- `ch <- value` → **gửi** value vào channel
- `value := <-ch` → **nhận** value từ channel

---

## Bước 6 — Đăng ký nhận Signal

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
```

**Các signal phổ biến:**

| Signal | Số | Ý nghĩa |
|---|---|---|
| `SIGINT` | 2 | Ctrl+C — "interrupt" |
| `SIGTERM` | 15 | Terminate lịch sự — Docker/K8s gửi |
| `SIGKILL` | 9 | Chết ngay — **không bắt được**, OS giết tức thì |
| `SIGHUP` | 1 | Reload config (truyền thống) |

Buffer size 1 để tránh miss signal trong race condition. Nếu unbuffered, lúc OS gửi signal mà chưa kịp đọc → signal bị drop.

**Cơ chế Graceful Shutdown:**

1. K8s muốn restart pod → gửi `SIGTERM`
2. App có ~30s để cleanup: đóng DB, finish request đang xử lý, flush log...
3. Sau 30s nếu chưa chết → K8s gửi `SIGKILL` → chết tức thì (có thể mất data)

> NestJS có `app.enableShutdownHooks()` làm tự động việc này, ẩn syscall đi. Go thì lộ rõ — bạn tự code, hiểu rõ chuyện gì xảy ra.

---

## Bước 7 — Đợi Signal hoặc Server Crash (`select`)

```go
select {
case sig := <-sigCh:
    log.Info("signal received", "signal", sig.String())
case err := <-errCh:
    log.Error("server failed", "err", err)
    os.Exit(1)
}
```

`select` là switch-case **dành riêng cho channel**. Đợi event nào tới trước thì chạy case đó. Nếu nhiều case sẵn sàng cùng lúc, Go chọn **ngẫu nhiên** một case (fairness).

Logic:
- **Case 1:** nhận signal → log, tiếp tục xuống dưới shutdown bình thường
- **Case 2:** server crash trước (`Run()` trả error) → log + exit ngay

> Biến `err` trong case 2 **shadow** biến `err` ở scope ngoài. Scope của nó chỉ trong case này — pattern thường thấy trong Go.

---

## Bước 8 — Graceful Shutdown với Timeout

**`context.WithTimeout`:**

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
```

Tạo context con từ context cha, tự động cancel sau 10s. Ý nghĩa: "Tao cho mày tối đa 10s để dọn dẹp, không xong tao cũng đi".

`context.Background()` là context **gốc** — không bao giờ tự cancel, không có deadline. Dùng làm parent khi tạo context con, thường dùng ở `main()`, test, hoặc khi bắt đầu request.

`10*time.Second`: `time.Second` là constant kiểu `time.Duration` (= 1 tỷ nanosecond). Nhân 10 = 10 giây.

**`defer` — keyword đặc biệt của Go:**

"Chạy hàm này **khi hàm hiện tại return** (dù return bình thường hay panic)".

> Tương đương `finally` trong JS nhưng linh hoạt hơn: `defer` đặt ngay sau khi acquire resource → gần và dễ đọc.

Nếu nhiều `defer` trong 1 hàm: chạy theo thứ tự **LIFO** (Last In First Out).

**Best practice:** hễ tạo `context.WithTimeout/WithCancel/WithDeadline` thì `defer cancel()` ngay để tránh leak goroutine ngầm bên trong context.

**If với init clause:**

```go
if err := a.Shutdown(ctx); err != nil {
```

Khai báo biến **ngay trong if**. `err` chỉ tồn tại trong scope của if/else. Code gọn, không "rò rỉ" biến ra ngoài. **Pattern cực kỳ phổ biến trong Go — phải làm quen.**

Shutdown fail có thể vì:
- Quá 10s vẫn chưa xong → ctx timeout
- 1 component panic khi đóng
- Resource bị lỗi khi đóng

---

## Kết thúc — Shutdown thành công

```go
log.Info("server stopped")
```

Tới đây = mọi thứ cleanup gọn gàng. Hàm `main()` return → process exit code 0. K8s/Docker thấy exit 0 → biết app dừng đúng cách, không restart.

---

## Tổng kết — Các khái niệm OS-level trong file này

| Khái niệm | Ý nghĩa |
|---|---|
| **OS** | Phần mềm trung gian giữa code và phần cứng |
| **stdout/stderr** | 2 luồng output mặc định OS cấp cho mọi process |
| **Env vars** | Cấu hình OS truyền vào process khi chạy |
| **Signal** | Cơ chế OS "ra lệnh" cho process (SIGINT/SIGTERM/SIGKILL) |
| **Exit code** | Số process trả về OS khi chết (0 = OK, ≠0 = lỗi) |
| **Syscall** | Lệnh gọi vào kernel của OS |
| **Log aggregator** | Hệ thống gom log từ nhiều server về 1 chỗ |
| **Goroutine** | Đơn vị thực thi đa luồng siêu nhẹ của Go |
| **Channel** | Ống dẫn thread-safe giữa các goroutine |
| **Context** | Cơ chế truyền tín hiệu hủy/timeout xuyên call chain |
| **Pointer (`&` và `*`)** | Địa chỉ memory — Go bắt rõ, JS che giấu |
| **Defer** | Chạy hàm khi function hiện tại kết thúc (giống finally) |

> NestJS/Node che giấu hầu hết những thứ này qua các abstraction (`process` global, EventEmitter, async/await, `app.enableShutdownHooks`). Go thì lộ rõ ra — học khó hơn nhưng hiểu sâu hơn về system programming.