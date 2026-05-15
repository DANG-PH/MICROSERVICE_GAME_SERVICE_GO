# game-service-go — Protobuf Protocol

## Bắt đầu

Xem **[SETUP.md](./SETUP.md)** để cài đặt môi trường (Go, Make, Air, protoc).

Sau khi setup xong:
```bash
make proto-win   # Windows — generate pb code
make proto       # Linux / macOS

go mod tidy      # resolve dependencies
make dev         # chạy server với hot-reload
```

---

## Cấu trúc file

```
game-service-go/
├── proto/
│   └── game.proto                ← Schema — tự viết, sửa ở đây
├── internal/
│   └── protocol/
│       └── pb/
│           └── game.pb.go        ← AUTO-GENERATED — không edit tay
└── go.mod
```

---

## go install vs go get

| | `go install` | `go get` |
|---|---|---|
| Dùng cho | CLI tool / binary | Library / dependency |
| Kết quả | File .exe trong `$GOPATH/bin` | Thêm vào `go.mod` + `go.sum` |
| Ví dụ | `protoc-gen-go` (plugin chạy lúc generate) | `google.golang.org/protobuf` (import trong code) |

**Sub-package không cần get riêng** — `game.pb.go` tự import `protoimpl`, `protoreflect`...
Tất cả đều nằm trong module `google.golang.org/protobuf`, `go get` 1 lần là đủ, `go mod tidy` tự resolve phần còn lại.

---

## Workflow khi sửa proto

```bash
# 1. Sửa proto/game.proto
# 2. Generate lại
make proto-win   # Windows
make proto       # Linux / macOS

# 3. Resolve dependencies mới (nếu có)
go mod tidy
```

---

## Dùng trong code Go

### Import
```go
import (
    pb  "github.com/DANG-PH/game-service-go/internal/protocol/pb"
    "google.golang.org/protobuf/proto"
)
```

### Nhận message từ client (WebSocket handler)
```go
var env pb.Envelope
if err := proto.Unmarshal(rawBytes, &env); err != nil {
    return
}

switch p := env.Payload.(type) {
case *pb.Envelope_Handshake:
    hs := p.Handshake
    // hs.ProtocolVersion, hs.UserId, hs.Token, hs.GameSessionId

case *pb.Envelope_PlayerMove:
    pm := p.PlayerMove
    // pm.MapId, pm.X, pm.Y, pm.Trangthai, pm.Dir
    // pm.Dau, pm.Than, pm.ChanField
    // pm.DangMangVanBay, pm.TenVanBay, pm.Rong, pm.Cao, pm.Avatar
}
```

### Gửi HandshakeAck
```go
msg := &pb.Envelope{
    Payload: &pb.Envelope_HandshakeAck{
        HandshakeAck: &pb.HandshakeAck{},
    },
}
data, _ := proto.Marshal(msg)
conn.WriteMessage(websocket.BinaryMessage, data)
```

### Gửi HandshakeNack
```go
msg := &pb.Envelope{
    Payload: &pb.Envelope_HandshakeNack{
        HandshakeNack: &pb.HandshakeNack{
            Reason: pb.NackReason_NACK_REASON_VERSION,
            // hoặc: NACK_REASON_AUTH, NACK_REASON_SESSION, NACK_REASON_INTERNAL
        },
    },
}
data, _ := proto.Marshal(msg)
conn.WriteMessage(websocket.BinaryMessage, data)
```

### Gửi PlayerSyncBatch (broadcast tick)
```go
pbPlayers := make([]*pb.PlayerSync, len(dirtyPlayers))
for i, p := range dirtyPlayers {
    pbPlayers[i] = &pb.PlayerSync{
        UserId:         p.UserID,
        X:              p.X,
        Y:              p.Y,
        Trangthai:      uint32(p.Trangthai),
        Dir:            int32(p.Dir),
        Dau:            p.Dau,
        Than:           p.Than,
        ChanField:      p.Chan,       // ← "chan" là keyword Go nên đổi thành ChanField
        TimeChoHienBay: p.TimeChoHienBay,
        LechDauX:       p.LechDauX,
        LechDauY:       p.LechDauY,
        LechThanX:      p.LechThanX,
        LechThanY:      p.LechThanY,
        LechChanX:      p.LechChanX,
        LechChanY:      p.LechChanY,
        DangMangVanBay: p.DangMangVanBay,
        TenVanBay:      p.TenVanBay,
        Rong:           p.Rong,
        Cao:            p.Cao,
        Avatar:         p.Avatar,
        ServerTime:     p.ServerTime,
    }
}

msg := &pb.Envelope{
    Payload: &pb.Envelope_PlayerSyncBatch{
        PlayerSyncBatch: &pb.PlayerSyncBatch{Players: pbPlayers},
    },
}
data, _ := proto.Marshal(msg)
```

---

## Lưu ý quan trọng

| Custom binary cũ | Protobuf Go |
|---|---|
| `Chan string` | `ChanField` — "chan" là keyword Go |
| `byte đầu = msgType` | `switch env.Payload.(type)` |
| `NackReasonVersion = 1` | `pb.NackReason_NACK_REASON_VERSION` |
| `uint8 Trangthai` | `uint32` — proto không có uint8 |
| `int8 Dir` | `int32` — proto không có int8 |

---

## Phía LibGDX (Java client) — dùng cùng file .proto

```bash
# build.gradle
implementation 'com.google.protobuf:protobuf-java:3.25.0'

# Generate Java code từ cùng file proto
protoc --java_out=./src/main/java proto/game.proto
```

```java
// Gửi
Envelope env = Envelope.newBuilder()
    .setHandshake(Handshake.newBuilder()
        .setProtocolVersion(1)
        .setUserId(userId)
        .setToken(token)
        .setGameSessionId(sessionId)
        .build())
    .build();
conn.send(env.toByteArray());

// Nhận
Envelope env = Envelope.parseFrom(rawBytes);
if (env.hasPlayerSyncBatch()) {
    for (PlayerSync p : env.getPlayerSyncBatch().getPlayersList()) {
        // p.getUserId(), p.getX(), p.getY()...
    }
}
```