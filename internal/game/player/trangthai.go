package player

// Trangthai enum - PHẢI khớp với Java enum TrangThai.java
// Thứ tự ordinal quan trọng vì lưu thành byte.
//
// THAY ĐỔI SO VỚI CUSTOM BINARY:
//   - byte → uint32 để khớp với kiểu proto (proto không có uint8/byte)
//   - Logic và giá trị string giữ nguyên hoàn toàn → NestJS/Redis không bị ảnh hưởng
const (
	TrangthaiDungYen  uint32 = iota // 0
	TrangthaiDiChuyen               // 1
	TrangthaiNhay                   // 2
	TrangthaiRoi                    // 3
	TrangthaiBayNgang               // 4
	TrangthaiThu                    // 5
	TrangthaiGong                   // 6
)

var trangthaiToString = map[uint32]string{
	TrangthaiDungYen:  "DUNG_YEN",
	TrangthaiDiChuyen: "DI_CHUYEN",
	TrangthaiNhay:     "NHAY",
	TrangthaiRoi:      "ROI",
	TrangthaiBayNgang: "BAY_NGANG",
	TrangthaiThu:      "THU",
	TrangthaiGong:     "GONG",
}

var stringToTrangthai = map[string]uint32{
	"DUNG_YEN":  TrangthaiDungYen,
	"DI_CHUYEN": TrangthaiDiChuyen,
	"NHAY":      TrangthaiNhay,
	"ROI":       TrangthaiRoi,
	"BAY_NGANG": TrangthaiBayNgang,
	"THU":       TrangthaiThu,
	"GONG":      TrangthaiGong,
}

func TrangthaiToString(v uint32) string {
	if s, ok := trangthaiToString[v]; ok {
		return s
	}
	return "DUNG_YEN"
}

func StringToTrangthai(s string) uint32 {
	if v, ok := stringToTrangthai[s]; ok {
		return v
	}
	return TrangthaiDungYen
}
