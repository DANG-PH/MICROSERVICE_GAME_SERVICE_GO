package player

// Trangthai enum - PHẢI khớp với Java enum TrangThai.java
// Thứ tự ordinal quan trọng vì lưu thành byte.
const (
	TrangthaiDungYen  uint8 = 0
	TrangthaiDiChuyen uint8 = 1
	TrangthaiNhay     uint8 = 2
	TrangthaiRoi      uint8 = 3
	TrangthaiBayNgang uint8 = 4
	TrangthaiThu      uint8 = 5
	TrangthaiGong     uint8 = 6
)

var trangthaiToString = map[uint8]string{
	TrangthaiDungYen:  "DUNG_YEN",
	TrangthaiDiChuyen: "DI_CHUYEN",
	TrangthaiNhay:     "NHAY",
	TrangthaiRoi:      "ROI",
	TrangthaiBayNgang: "BAY_NGANG",
	TrangthaiThu:      "THU",
	TrangthaiGong:     "GONG",
}

var stringToTrangthai = map[string]uint8{
	"DUNG_YEN":  TrangthaiDungYen,
	"DI_CHUYEN": TrangthaiDiChuyen,
	"NHAY":      TrangthaiNhay,
	"ROI":       TrangthaiRoi,
	"BAY_NGANG": TrangthaiBayNgang,
	"THU":       TrangthaiThu,
	"GONG":      TrangthaiGong,
}

func TrangthaiToString(v uint8) string {
	if s, ok := trangthaiToString[v]; ok {
		return s
	}
	return "DUNG_YEN" // fallback
}

func StringToTrangthai(s string) uint8 {
	if v, ok := stringToTrangthai[s]; ok {
		return v
	}
	return TrangthaiDungYen
}
