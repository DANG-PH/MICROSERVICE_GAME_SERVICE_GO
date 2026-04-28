// ============================================================================
// PACKAGE DECLARATION
// ============================================================================
// "package main" - khai báo BẮT BUỘC ở đầu mọi file Go.
// Tên "main" có ý nghĩa ĐẶC BIỆT: báo Go đây là chương trình chạy được (executable),
// không phải library. Mọi file khác trong cùng folder cũng phải là "package main".
//
// So với NestJS: NestJS không có khái niệm package, chỉ có module. File main.ts
// là entry point ngầm định, không cần khai báo gì. Go thì rõ ràng hơn:
// "main" + hàm main() = chương trình chạy được.
package main

// ============================================================================
// IMPORT BLOCK
// ============================================================================
// Go gom mọi import vào trong ngoặc (). Convention chia 3 nhóm cách nhau dòng trống:
//   1. Standard library (có sẵn trong Go - không cần cài)
//   2. Third-party packages (từ GitHub - cài qua "go get")
//   3. Internal packages (code project của mình)
// Tool "goimports" tự động sắp xếp và xóa import thừa.
import (
	// --- STANDARD LIBRARY ---

	// "context" package: cơ chế truyền tín hiệu HỦY BỎ và DEADLINE xuyên suốt call chain.
	// Tưởng tượng: 1 request HTTP đi qua 5 layer (handler → service → repo → DB → cache).
	// Nếu user hủy request, bạn muốn TẤT CẢ 5 layer dừng ngay để khỏi lãng phí tài nguyên.
	// Context truyền "lệnh dừng" xuống tất cả các tầng.
	// NestJS KHÔNG có khái niệm này built-in - gần nhất là AbortController của JS.
	"context"

	// "log/slog": Structured Logging (từ Go 1.21+).
	// Structured = log dạng key-value (JSON), KHÔNG phải string thường.
	// Vì sao? Production có hàng trăm server, log gom về log aggregator
	// (ELK/Loki/Datadog...) - JSON parse nhanh hơn regex parse text 100 lần.
	// Tương đương: Pino/Winston bên Node, nhưng slog là STANDARD nên không cần cài.
	"log/slog"

	// "os" package: cầu nối nói chuyện với HỆ ĐIỀU HÀNH (Operating System).
	// OS là phần mềm trung gian giữa code của bạn và phần cứng (CPU/RAM/disk/network).
	// Khi bạn chạy app, OS cấp RAM, cho mở file, mở socket, đọc env vars,
	// gửi signal khi muốn dừng app...
	// "os" giúp bạn:
	//   - os.Getenv("KEY")   → đọc biến môi trường
	//   - os.Exit(1)         → thoát chương trình với mã lỗi
	//   - os.Stdout          → luồng output chuẩn
	//   - os.Args            → tham số dòng lệnh
	// NestJS che giấu cái này qua "process" global của Node:
	//   process.env  ↔  os.Getenv
	//   process.exit ↔  os.Exit
	//   process.stdout ↔ os.Stdout
	// Bản chất giống hệt, chỉ là Node wrap sẵn còn Go bắt import thủ công.
	"os"

	// "os/signal": bắt SIGNAL từ OS.
	// Signal là cách OS "ra lệnh" cho process đang chạy. Ví dụ:
	//   - Bạn nhấn Ctrl+C trong terminal → terminal gửi SIGINT đến app
	//   - Docker chạy "docker stop" → Docker gửi SIGTERM đến container
	//   - K8s xóa pod → K8s gửi SIGTERM, đợi 30s rồi gửi SIGKILL
	// App có thể BẮT signal để cleanup trước khi chết (graceful shutdown).
	"os/signal"

	// "syscall": System Calls - các lệnh GỌI THẲNG vào kernel của OS.
	// "Kernel" là phần lõi của OS - phần mềm có quyền tối cao trên máy.
	// Bình thường app chạy ở "user space" (giới hạn quyền), khi cần tương tác
	// với phần cứng/file/network phải gọi syscall vào "kernel space".
	// Ở đây ta chỉ dùng syscall để LẤY HẰNG SỐ định danh signal:
	//   - syscall.SIGINT  = số 2 = Ctrl+C
	//   - syscall.SIGTERM = số 15 = lệnh terminate lịch sự
	"syscall"

	// "time": xử lý thời gian, duration, timezone, timer...
	// Ở đây dùng để tạo timeout 10 giây.
	// Tương đương: Date + setTimeout của JS gộp lại.
	"time"

	// --- THIRD-PARTY PACKAGE ---

	// "godotenv": load file .env vào env vars của process.
	// File .env chứa cấu hình kiểu: DATABASE_URL=postgres://...
	// Lib này đọc file → set vào os.Getenv → code đọc bằng os.Getenv("DATABASE_URL").
	// Tương đương: lib "dotenv" bên Node mà NestJS hay dùng (qua @nestjs/config).
	"github.com/joho/godotenv"

	// --- INTERNAL PACKAGES (CODE CỦA PROJECT MÌNH) ---

	// "internal/" là FOLDER ĐẶC BIỆT trong Go: chỉ package CÙNG MODULE mới import được.
	// Giống "private" ở mức module - bảo vệ không cho project khác import nhầm.
	// Ví dụ: project A có "internal/secret" → project B KHÔNG import được.
	// Đây là cơ chế module-level encapsulation. NestJS không có cơ chế này -
	// mọi thứ export đều public.

	// Package config: chứa logic load + validate cấu hình từ env vars.
	"github.com/DANG-PH/game-service-go/internal/config"

	// Package app: chứa logic khởi tạo + chạy + tắt ứng dụng (HTTP server, DB pool...).
	// Tương đương AppModule + main bootstrap logic của NestJS gộp lại.
	"github.com/DANG-PH/game-service-go/internal/app"
)

// ============================================================================
// HÀM MAIN - ENTRY POINT
// ============================================================================
// "func main()": entry point của chương trình. BẮT BUỘC:
//   - Nằm trong package main
//   - Tên "main", không nhận tham số, không return
//
// Khi chạy "./my-app", Go runtime gọi main() đầu tiên. Khi main() return → chương trình kết thúc.
//
// So với NestJS: tương đương async function bootstrap() {...} bootstrap() trong main.ts.
// Khác biệt: NestJS dùng async/await (event loop single-thread), Go dùng goroutine + channel
// (concurrency thật sự song song trên nhiều CPU core).
func main() {
	// ========================================================================
	// BƯỚC 1: LOAD FILE .ENV (nếu có)
	// ========================================================================
	// godotenv.Load() đọc file .env ở thư mục hiện tại, parse từng dòng KEY=VALUE,
	// rồi set vào env vars của process (qua os.Setenv ngầm).
	// Sau khi load xong, code có thể đọc bằng os.Getenv("KEY").
	//
	// Vì sao "_ = godotenv.Load()"?
	// - Hàm Load() trả về error nếu file .env không tồn tại.
	// - Nhưng ở production (Docker/K8s) ta KHÔNG dùng file .env - env vars được
	//   inject qua docker run -e hoặc K8s ConfigMap/Secret.
	// - Local dev mới có file .env. Vậy nếu thiếu file thì kệ, không sao.
	// - Dấu "_" gọi là BLANK IDENTIFIER, ý: "tao biết hàm này có return value
	//   nhưng tao cố tình bỏ qua".
	//
	// Vì sao phải có "_"? Go có quy tắc CỰC NGHIÊM:
	// MỌI biến khai báo PHẢI dùng, MỌI return value PHẢI xử lý hoặc bỏ qua tường minh.
	// Khác hẳn JS/TS - vốn cho phép ignore tự do (đôi khi dẫn đến bug ngầm).
	_ = godotenv.Load()

	// ========================================================================
	// BƯỚC 2: KHỞI TẠO LOGGER
	// ========================================================================
	// Tạo structured logger ghi log JSON ra stdout.
	//
	// "stdout" (Standard Output) là gì?
	// - Mỗi process khi chạy có 3 luồng (stream) mặc định OS cấp:
	//     stdin  (0): nhận input - VD: bạn gõ phím vào terminal
	//     stdout (1): output bình thường - VD: console.log, fmt.Println
	//     stderr (2): output lỗi/log - VD: console.error
	// - Tách stdout/stderr để có thể chuyển hướng riêng:
	//     ./my-app > out.log 2> err.log    # bash redirect
	// - Trong K8s/Docker: container engine TỰ ĐỘNG hứng stdout/stderr của app
	//   rồi forward đến hệ thống log. Vì vậy production luôn log ra stdout
	//   chứ KHÔNG ghi vào file.
	//
	// "Log Aggregator" là gì?
	// - "Aggregate" = gom lại, tổng hợp.
	// - Có 50 con server, mỗi con in log ra stdout. Chẳng lẽ SSH vào từng con đọc?
	// - Giải pháp: agent ở mỗi server đọc stdout → gửi về 1 server trung tâm
	//   (aggregator) → bạn vào dashboard 1 chỗ là thấy log của TẤT CẢ.
	// - Tools phổ biến: ELK, Loki+Grafana, Datadog, Splunk, CloudWatch.
	// - JSON log nhanh hơn text log gấp nhiều lần khi parse → query dễ dàng.
	//
	// Cú pháp slog.New(handler):
	// - slog.NewJSONHandler(writer, options) tạo handler ghi JSON vào writer.
	// - "writer" ở đây là os.Stdout (đã giải thích ở trên).
	// - "&slog.HandlerOptions{...}" - dấu "&" lấy ĐỊA CHỈ (pointer) của struct.
	//   Vì sao cần pointer? Vì hàm NewJSONHandler nhận *HandlerOptions chứ không
	//   nhận HandlerOptions. Truyền pointer = truyền địa chỉ memory, không copy.
	//   Trong NestJS không cần nghĩ vì JS object mặc định by reference.
	//
	// Level: slog.LevelInfo
	// - slog có 4 level: Debug < Info < Warn < Error.
	// - Set Info nghĩa là CHỈ log từ Info trở lên (Info/Warn/Error), bỏ Debug.
	// - Production tránh log Debug vì quá nhiều, tốn tiền log aggregator.
	// - Local dev có thể đổi sang slog.NewTextHandler để log text dễ đọc hơn JSON.
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Set logger này làm DEFAULT global - các package khác gọi slog.Info(...)
	// (không qua biến log) sẽ tự động dùng logger này.
	// Pattern singleton. NestJS có Logger class behavior tương tự.
	slog.SetDefault(log)

	// ========================================================================
	// BƯỚC 3: LOAD CONFIG
	// ========================================================================
	// config.Load() là hàm tự viết trong package config. Thường nó:
	//   1. Đọc env vars (PORT, DATABASE_URL...) qua os.Getenv
	//   2. Validate (port phải là số, DB URL phải đúng format...)
	//   3. Trả về struct Config + error
	//
	// Cú pháp ":=" - SHORT VARIABLE DECLARATION:
	// - "cfg, err := config.Load()" tương đương:
	//     var cfg ConfigType
	//     var err error
	//     cfg, err = config.Load()
	//   nhưng gọn hơn nhiều. Go TỰ SUY RA TYPE từ vế phải.
	// - Giống "const cfg = ..." trong TS, nhưng Go có thể reassign sau (như "let").
	//
	// MULTIPLE RETURN VALUES - đặc sản của Go:
	// - Hàm Go có thể trả về NHIỀU giá trị: (Config, error) chẳng hạn.
	// - Đây là pattern xử lý lỗi CHUẨN của Go: thay vì throw exception như NestJS,
	//   Go trả về error như giá trị thường. Ai gọi hàm phải tự check.
	// - Ưu điểm: tường minh, nhìn code biết ngay chỗ nào có thể lỗi.
	// - Nhược điểm: code dài hơn vì nhiều "if err != nil".
	cfg, err := config.Load()

	// Pattern check error CHUẨN của Go - sẽ thấy nó hàng chục lần trong project.
	// "err != nil" nghĩa là CÓ lỗi (nil = null/None bên ngôn ngữ khác).
	if err != nil {
		// Log error với key "err" - structured logging cho phép query/filter sau này.
		// VD trong Loki: {service="game"} | json | err != ""
		log.Error("config load failed", "err", err)

		// os.Exit(1): thoát chương trình NGAY LẬP TỨC với mã lỗi 1.
		// Quy ước Unix:
		//   exit 0 = thành công
		//   exit != 0 = lỗi (thường là 1, đôi khi mã lỗi cụ thể)
		// CẢNH BÁO QUAN TRỌNG: os.Exit KHÔNG chạy defer (sẽ giải thích defer dưới).
		// Ở main() lúc này chưa có resource nào cần cleanup nên OK.
		// Nhưng đừng bao giờ os.Exit ở giữa hàm có defer - sẽ leak resource.
		os.Exit(1)
	}

	// ========================================================================
	// BƯỚC 4: KHỞI TẠO APP
	// ========================================================================
	// app.New(cfg, log) là constructor function tự viết.
	// Tương đương NestFactory.create(AppModule) trong NestJS.
	// Bên trong thường:
	//   1. Tạo HTTP server (Gin/Echo/chi/net.http...)
	//   2. Connect DB (Postgres/MySQL...)
	//   3. Connect Redis/Cache
	//   4. Wire dependencies (DI thủ công vì Go không có @Injectable)
	//   5. Đăng ký routes/middleware
	//
	// Quy ước Go: hàm tạo instance đặt tên "New..." hoặc "NewXxx".
	// Go KHÔNG có class/constructor - chỉ có struct + hàm thường.
	// Đây là khác biệt lớn so với NestJS (vốn rất OOP).
	a, err := app.New(cfg, log)
	if err != nil {
		log.Error("app init failed", "err", err)
		os.Exit(1)
	}

	// ========================================================================
	// BƯỚC 5: CHẠY APP TRONG GOROUTINE
	// ========================================================================
	// PHẦN CỐT LÕI: Concurrency với GOROUTINE + CHANNEL.
	//
	// VẤN ĐỀ: a.Run() khởi động HTTP server. Server lắng nghe forever (blocking).
	// Nếu chạy thẳng "a.Run()" thì code SAU nó không bao giờ được chạy
	// → không có cơ chế graceful shutdown.
	// → Phải chạy trong goroutine để main thread tiếp tục đợi signal.
	//
	// CHANNEL là gì?
	// - "Ống dẫn" thread-safe để các goroutine gửi/nhận data.
	// - Triết lý của Go: "Don't communicate by sharing memory; share memory by communicating"
	//   (đừng chia sẻ biến rồi đặt lock, hãy gửi data qua channel).
	//
	// Cú pháp make(chan Type, bufferSize):
	// - "chan error" - channel chứa giá trị kiểu error
	// - Số 1 - BUFFER SIZE = 1 phần tử
	// - Buffered (>0): gửi không block đến khi đầy buffer
	// - Unbuffered (=0 hoặc bỏ trống): gửi BLOCK đến khi có người nhận
	//
	// Ở đây buffer 1 vì: a.Run() có thể return error MỘT LẦN, ta muốn nó gửi xong
	// luôn rồi goroutine kết thúc, không bị treo chờ ai đó nhận.
	errCh := make(chan error, 1)

	// "go func() {...}()": tạo GOROUTINE.
	//
	// GOROUTINE là gì?
	// - Đơn vị thực thi siêu nhẹ của Go (~2KB stack ban đầu, thread OS thường 1-8MB).
	// - Có thể tạo HÀNG TRIỆU goroutine cùng lúc.
	// - Go scheduler tự phân phối goroutine lên các OS thread → CHẠY THẬT SỰ
	//   SONG SONG trên nhiều CPU core.
	//
	// SO SÁNH VỚI NESTJS/NODE:
	// - Node JS chạy SINGLE-THREADED trên event loop. await fetch() không thực sự
	//   đợi - nó đăng ký callback rồi đi làm việc khác. Vẫn chỉ 1 thread.
	// - Go goroutine chạy ĐA LUỒNG thật. Máy 8 core có thể chạy 8 goroutine đồng thời.
	// - Đây là PARADIGM SHIFT khi từ Node sang Go.
	//
	// "func() {...}()" là IIFE (Immediately Invoked Function Expression) - giống JS.
	// Cặp "()" cuối là gọi hàm ngay sau khi định nghĩa.
	go func() {
		// Toán tử CHANNEL:
		//   "ch <- value"   → GỬI value vào channel
		//   "value := <-ch" → NHẬN value từ channel
		//
		// Ở đây: chạy a.Run() (blocking forever), khi nó return error
		// (do server crash hoặc shutdown) thì gửi error đó vào errCh.
		errCh <- a.Run()
	}()

	// ========================================================================
	// BƯỚC 6: ĐĂNG KÝ NHẬN SIGNAL
	// ========================================================================
	// Tạo channel để nhận signal từ OS.
	// os.Signal là INTERFACE đại diện cho signal hệ điều hành.
	//
	// SIGNAL là gì? (chi tiết hơn)
	// - Signal là CƠ CHẾ OS GỬI LỆNH cho process đang chạy.
	// - Khi bạn nhấn Ctrl+C, terminal KHÔNG tự kill process - nó gửi SIGINT
	//   đến process, process tự quyết định làm gì.
	// - Các signal phổ biến:
	//     SIGINT  (2)  = Ctrl+C - "interrupt"
	//     SIGTERM (15) = "terminate lịch sự" - Docker/K8s gửi
	//     SIGKILL (9)  = "chết ngay" - KHÔNG bắt được, OS giết tức thì
	//     SIGHUP  (1)  = reload config (truyền thống)
	//
	// Buffer size 1: tránh MISS signal trong race condition.
	// Nếu unbuffered, lúc OS gửi signal mà ta chưa kịp đọc → signal bị drop.
	sigCh := make(chan os.Signal, 1)

	// signal.Notify ĐĂNG KÝ với OS:
	// "Khi process này nhận SIGINT hoặc SIGTERM, đẩy vào sigCh".
	//
	// Vì sao chỉ bắt SIGINT/SIGTERM?
	// - SIGINT: dev local nhấn Ctrl+C
	// - SIGTERM: production (Docker/K8s) muốn dừng container "lịch sự"
	// - SIGKILL không bắt được - kernel giết process ngay, không cho cleanup
	//
	// CƠ CHẾ GRACEFUL SHUTDOWN:
	// 1. K8s muốn restart pod → gửi SIGTERM
	// 2. App có ~30s để cleanup: đóng DB, finish request đang xử lý, flush log...
	// 3. Sau 30s nếu chưa chết → K8s gửi SIGKILL → chết tức thì (có thể mất data)
	//
	// NestJS có app.enableShutdownHooks() làm tự động việc này, ẩn syscall đi.
	// Go thì lộ rõ - bạn tự code, hiểu rõ chuyện gì xảy ra.
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// ========================================================================
	// BƯỚC 7: ĐỢI SIGNAL HOẶC SERVER CRASH (SELECT)
	// ========================================================================
	// "select" là switch-case DÀNH RIÊNG cho channel.
	// Đợi event nào tới TRƯỚC thì chạy case đó, các case khác bị bỏ qua.
	// Nếu nhiều case sẵn sàng cùng lúc, Go chọn NGẪU NHIÊN một case (fairness).
	//
	// Logic:
	//   Case 1: nhận signal → log, tiếp tục xuống dưới shutdown bình thường
	//   Case 2: server crash trước (Run() trả error) → log + exit ngay
	//
	// Đây là pattern QUAN TRỌNG cần nhớ - sẽ dùng đi dùng lại trong code Go.
	select {
	case sig := <-sigCh:
		// Nhận được signal. Biến sig kiểu os.Signal.
		// .String() convert thành tên dễ đọc: "interrupt" cho SIGINT,
		// "terminated" cho SIGTERM.
		log.Info("signal received", "signal", sig.String())

	case err := <-errCh:
		// Server fail TRƯỚC khi có signal - thường lỗi nghiêm trọng:
		//   - Port đã bị process khác chiếm
		//   - DB connect fail
		//   - Permission lỗi...
		//
		// LƯU Ý: biến "err" này SHADOW biến err ở scope ngoài.
		// Scope của nó CHỈ trong case này. Pattern thường thấy trong Go.
		log.Error("server failed", "err", err)
		os.Exit(1)
	}

	// ========================================================================
	// BƯỚC 8: GRACEFUL SHUTDOWN VỚI TIMEOUT
	// ========================================================================
	// Tới đây = đã nhận signal, cần shutdown LỊCH SỰ.
	//
	// context.WithTimeout(parent, duration):
	// - Tạo context CON từ context cha, tự động cancel sau "duration".
	// - Ý nghĩa: "Tao cho mày tối đa 10s để dọn dẹp, không xong tao cũng đi"
	//
	// context.Background() là context "GỐC":
	// - Không bao giờ tự cancel
	// - Không có deadline
	// - Dùng làm parent khi tạo context con
	// - Thường dùng ở main(), test, hoặc khi bắt đầu request
	//
	// Trả về:
	//   ctx    - context với deadline 10s
	//   cancel - hàm hủy context THỦ CÔNG (giải phóng resource sớm hơn deadline)
	//
	// 10*time.Second:
	// - time.Second là constant kiểu time.Duration (= 1 tỷ nanosecond)
	// - Nhân 10 = 10 giây
	// - Cú pháp này tự nhiên nhờ Go cho phép operator overload với Duration
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	// "defer" là KEYWORD ĐẶC BIỆT của Go:
	// "Chạy hàm này KHI HÀM HIỆN TẠI return (dù return bình thường hay panic)".
	//
	// Tương đương "finally" trong JS nhưng LINH HOẠT hơn:
	// - JS: try/finally bọc 1 đoạn code
	// - Go: defer đặt NGAY SAU khi acquire resource → gần và dễ đọc
	//
	// Nếu nhiều defer trong 1 hàm: chạy theo thứ tự LIFO (Last In First Out).
	//
	// BEST PRACTICE: hễ tạo context.WithTimeout/WithCancel/WithDeadline
	// thì DEFER cancel() NGAY để tránh leak goroutine ngầm bên trong context.
	defer cancel()

	// Gọi a.Shutdown(ctx) - app tự xử lý:
	//   - Đóng HTTP server (không nhận request mới)
	//   - Đợi request đang xử lý hoàn thành
	//   - Đóng DB pool
	//   - Đóng Redis connection
	//   - Flush metrics/log buffer...
	//
	// Truyền ctx xuống để Shutdown biết khi nào hết thời gian phải bỏ cuộc.
	//
	// Cú pháp "if err := X; err != nil":
	// - "IF STATEMENT WITH INIT CLAUSE" - khai báo biến NGAY trong if
	// - err CHỈ tồn tại trong scope của if/else
	// - Code gọn, không "rò rỉ" biến ra ngoài
	// - Pattern CỰC kỳ phổ biến trong Go - phải làm quen
	if err := a.Shutdown(ctx); err != nil {
		// Shutdown fail có thể vì:
		//   - Quá 10s vẫn chưa xong → ctx timeout
		//   - 1 component panic khi đóng
		//   - Resource bị lỗi khi đóng
		log.Error("shutdown failed", "err", err)
		os.Exit(1)
	}

	// ========================================================================
	// KẾT THÚC: SHUTDOWN THÀNH CÔNG
	// ========================================================================
	// Tới đây = mọi thứ cleanup gọn gàng. Hàm main() return → process exit code 0.
	// K8s/Docker thấy exit 0 → biết app dừng đúng cách, không restart.
	log.Info("server stopped")
}

// ============================================================================
// TỔNG KẾT NHỮNG KHÁI NIỆM OS-LEVEL TRONG FILE NÀY
// ============================================================================
// 1. OS (Operating System): phần mềm trung gian giữa code và phần cứng
// 2. stdout/stderr: 2 luồng output mặc định OS cấp cho mọi process
// 3. Env vars: cấu hình OS truyền vào process khi chạy
// 4. Signal: cơ chế OS "ra lệnh" cho process (SIGINT/SIGTERM/SIGKILL)
// 5. Exit code: số process trả về OS khi chết (0 = OK, !=0 = lỗi)
// 6. Syscall: lệnh gọi vào kernel của OS
// 7. Log aggregator: hệ thống gom log từ nhiều server về 1 chỗ
// 8. Goroutine: đơn vị thực thi đa luồng siêu nhẹ của Go
// 9. Channel: ống dẫn thread-safe giữa các goroutine
// 10. Context: cơ chế truyền tín hiệu hủy/timeout xuyên call chain
// 11. Pointer (& và *): địa chỉ memory - Go bắt rõ, JS che giấu
// 12. Defer: chạy hàm khi function hiện tại kết thúc (giống finally)
//
// NestJS/Node che giấu hầu hết những thứ này qua các abstraction
// (process global, EventEmitter, async/await, app.enableShutdownHooks).
// Go thì lộ rõ ra - học khó hơn nhưng hiểu sâu hơn về system programming.
