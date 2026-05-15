# Setup môi trường — game-service-go

Đọc file này nếu bạn vừa clone project về và chưa biết cài gì.

---

## Windows

### 1. Cài Go
Vào https://go.dev/dl/ → tải file `.msi` mới nhất → chạy installer → Next hết.

Kiểm tra:
```powershell
go version
# go version go1.xx windows/amd64
```

### 2. Cài Make
Make không có sẵn trên Windows. Cài qua **Chocolatey**:

```powershell
# Mở PowerShell với quyền Administrator, chạy lệnh này để cài Chocolatey
Set-ExecutionPolicy Bypass -Scope Process -Force
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))

# Sau khi cài xong, mở PowerShell mới rồi cài Make
choco install make -y
```

Kiểm tra:
```powershell
make --version
```

### 3. Cài Air (hot-reload cho make dev)
```powershell
go install github.com/air-verse/air@latest
```

Thêm `C:\Users\<tên-máy-bạn>\go\bin` vào PATH (xem bước 5 để biết cách thêm PATH).

### 4. Cài protoc

1. Vào https://github.com/protocolbuffers/protobuf/releases
2. Tìm bản mới nhất, tải file `protoc-xx.x-win64.zip`
3. Giải nén vào `C:\protoc`
4. Thêm `C:\protoc\bin` vào PATH:
   - Nhấn Start → gõ **"Environment Variables"** → mở
   - Chọn **Path** trong phần "User variables" → Edit → New
   - Dán vào: `C:\protoc\bin`
   - OK hết → **mở PowerShell mới**

Kiểm tra:
```powershell
protoc --version
# libprotoc 26.x
```

### 5. Cài protoc-gen-go (plugin generate code Go)

```powershell
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

Thêm `C:\Users\<tên-máy-bạn>\go\bin` vào PATH (làm giống bước 4):
- Xem đường dẫn chính xác: `go env GOPATH`
- Thêm kết quả đó + `\bin` vào PATH

Kiểm tra:
```powershell
protoc-gen-go --version
# protoc-gen-go v1.34.x
```

### 6. Cài dependency Go

```powershell
go get google.golang.org/protobuf
```

### 7. Generate protobuf & chạy server

```powershell
make proto-win
go mod tidy
make dev
```

---

## Linux (Ubuntu / Debian)

### 1. Cài Go
Vào https://go.dev/dl/ → copy lệnh tải bản mới nhất cho Linux, ví dụ:
```bash
wget https://go.dev/dl/go1.xx.x.linux-amd64.tar.gz   # thay xx.x bằng version mới nhất
sudo tar -C /usr/local -xzf go1.xx.x.linux-amd64.tar.gz

# Thêm vào ~/.bashrc
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

Kiểm tra:
```bash
go version
```

### 2. Cài Make
```bash
sudo apt install make -y
```

### 3. Cài Air (hot-reload cho make dev)
```bash
go install github.com/air-verse/air@latest

# Thêm vào ~/.bashrc nếu chưa có
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc
```

### 4. Cài protoc
```bash
sudo apt install protobuf-compiler -y
```

Kiểm tra:
```bash
protoc --version
# libprotoc 3.x
```

> Nếu cần version mới hơn (apt hay cho bản cũ), tải thẳng từ GitHub:
> https://github.com/protocolbuffers/protobuf/releases → chọn file `linux-x86_64.zip`

### 5. Cài protoc-gen-go

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Thêm vào ~/.bashrc nếu chưa có
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc
```

Kiểm tra:
```bash
protoc-gen-go --version
```

### 6. Cài dependency Go

```bash
go get google.golang.org/protobuf
```

### 7. Generate protobuf & chạy server

```bash
make proto
go mod tidy
make dev
```

---

## macOS

### 1. Cài Homebrew (nếu chưa có)
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

### 2. Cài các tool
```bash
brew install go protobuf make
```

### 3. Cài Air (hot-reload cho make dev)
```bash
go install github.com/air-verse/air@latest

# Thêm vào ~/.zshrc
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.zshrc
source ~/.zshrc
```

### 4. Cài protoc-gen-go

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

Kiểm tra:
```bash
protoc-gen-go --version
```

### 5. Cài dependency Go

```bash
go get google.golang.org/protobuf
```

### 6. Generate protobuf & chạy server

```bash
make proto
go mod tidy
make dev
```

---

## Kiểm tra nhanh — chạy lần lượt, tất cả phải không báo lỗi

```bash
go version               # Go đã cài
make --version           # Make đã cài
air -v                   # Air đã cài
protoc --version         # protoc đã cài
protoc-gen-go --version  # plugin đã cài và có trong PATH
```

Sau đó generate và chạy server:
```bash
# Windows
make proto-win && go mod tidy && make dev

# Linux / macOS
make proto && go mod tidy && make dev
```

Nếu thấy Air khởi động và không có lỗi đỏ là xong.