// internal/ws/bus_iface.go (file mới)

package bus

import "context"

// BusInterface là contract chung cho cross-instance message bus.
// Implementation có thể là Redis Bus hoặc NATS Bus.
type BusInterface interface {
	SetHandler(h BusHandler)
	Start(ctx context.Context) error
	Stop()
	NodeID() string

	PublishBroadcast(ctx context.Context, mapID string, data []byte, excludeUserID int32) error
	PublishKickUser(ctx context.Context, userID int32) error
}

// BusHandler là interface mà Bus gọi vào khi nhận message từ instance khác.
// Hub implement interface này.
//
// Tại sao interface thay vì 3 callback function rời?
//   - Gom nhóm contract liên quan vào 1 chỗ — caller chỉ cần thấy "implement BusHandler"
//     là biết phải có 3 method gì
//   - Compile-time check: nếu Hub thiếu method hoặc sai signature → báo lỗi ngay tại
//     dòng `var _ BusHandler = (*Hub)(nil)`, không phải đợi runtime
//   - Mock cho test gọn hơn: 1 struct implement 3 method vs gán 3 lambda rời
//   - Pattern phổ biến trong Go production: stdlib (http.Handler, io.Reader),
//     các framework (gRPC interceptor, k8s controller).
type BusHandler interface {
	OnBroadcast(mapID string, data []byte, excludeUserID int32)
	OnKickUser(userID int32)
}

// Verify cả 2 implementation đều thỏa interface.
var _ BusInterface = (*Bus)(nil)     // Redis bus
var _ BusInterface = (*NATSBus)(nil) // NATS bus
