# Contributing to golang-base

Cảm ơn bạn đã quan tâm đến golang-base! Repo này là **core skeleton dùng chung** — mọi contribution đều được cân nhắc kỹ để giữ tính tối giản và generic.

---

## Nguyên tắc cốt lõi

Trước khi mở PR, hãy tự hỏi:

> *"Thứ này có phải service nào cũng cần không?"*

Nếu câu trả lời là **không chắc** — thì đừng thêm vào đây.

**Được chấp nhận:**
- Cải thiện structure, naming convention, hoặc tài liệu
- Fix bug trong phần đã có sẵn (config loader, middleware, Dockerfile...)
- Cải thiện CI pipeline hoặc Makefile
- Thêm vào `pkg/` những utility thực sự generic (không phụ thuộc business logic)

**Không được chấp nhận:**
- Bất kỳ web framework nào (Gin, Echo, Fiber...)
- ORM hoặc database driver cụ thể (GORM, sqlx, pgx...)
- Business logic dưới mọi hình thức
- Authentication/authorization implementation
- Thêm dependency không cần thiết vào `go.mod`

---

## Quy trình

### 1. Mở Issue trước

Với thay đổi lớn hơn fix typo, hãy mở issue trước để thảo luận. Tránh tình trạng viết cả đống code rồi PR bị reject vì không phù hợp với hướng đi của repo.

### 2. Fork và tạo branch

```bash
git clone https://github.com/<your-username>/golang-base.git
cd golang-base
git checkout -b <type>/<short-description>
```

Tên branch theo pattern:
```
feat/add-grpc-interceptor-example
fix/recovery-middleware-nil-panic
docs/clarify-external-client-rules
chore/update-golangci-config
```

### 3. Viết code

Tuân thủ đúng [quy tắc mở rộng trong README](README.md#-quy-tắc-mở-rộng). Nếu thêm file mới, đặt đúng vị trí theo bảng tổng hợp.

Chạy các check sau trước khi commit:

```bash
make fmt      # format code
make vet      # static analysis
make lint     # golangci-lint
make test     # chạy tests
```

Tất cả phải pass, không có warning mới được commit.

### 4. Commit

Dùng [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add recovery interceptor for gRPC server
fix: handle nil context in logger middleware
docs: add WebSocket transport example to README
chore: bump golangci-lint to v1.57
refactor: simplify config loader default handling
```

Mỗi commit làm **1 việc**. Không commit "fix stuff" hay "update".

### 5. Mở Pull Request

- Title theo format Conventional Commits
- Mô tả ngắn: tại sao thay đổi này cần thiết, không phải nó làm gì (code đã nói điều đó)
- Nếu có issue liên quan: `Closes #<number>`
- Không cần checklist dài dòng — CI sẽ tự kiểm tra

---

## Code style

Repo này không dùng framework nên không có convention đặc biệt ngoài Go standard:

- `gofmt` — bắt buộc, chạy qua `make fmt`
- `golangci-lint` với config trong `.golangci.yml` — bắt buộc
- Comment public API bằng tiếng Anh, theo Go doc convention (`// FunctionName does...`)
- Error message viết thường, không dấu chấm cuối (`"something went wrong"` không phải `"Something went wrong."`)
- Không dùng `panic` trong library code — chỉ dùng trong `main.go` khi startup fail

---

## Thêm vào `pkg/`

Đây là nơi hay bị lạm dụng nhất. Package mới vào `pkg/` phải đáp ứng **cả hai**:

1. Không chứa bất kỳ business logic nào
2. Có thể copy sang service khác và dùng ngay mà không cần sửa

Nếu chỉ đáp ứng 1 trong 2 — không thuộc về đây.

Mỗi package là 1 folder, file chính trùng tên package:
```
pkg/paginate/paginate.go    ✅
pkg/utils/string.go         ❌  — tên generic không được phép
pkg/auth/jwt.go             ❌  — business-specific
```

---

## Câu hỏi?

Mở [Discussion](https://github.com/DANG-PH/golang-base/discussions) hoặc comment thẳng vào issue liên quan.