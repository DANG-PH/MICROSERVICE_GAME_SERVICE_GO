package player

// Trangthai enum - PHẢI khớp với Java enum TrangThai.java
// Thứ tự ordinal quan trọng vì lưu thành byte.
const (
	TrangthaiDungYen  byte = iota // 0
	TrangthaiDiChuyen             // 1
	TrangthaiNhay                 // 2
	TrangthaiRoi                  // 3
	TrangthaiBayNgang             // 4
	TrangthaiThu                  // 5
	TrangthaiGong                 // 6
)

var trangthaiToString = map[byte]string{
	TrangthaiDungYen:  "DUNG_YEN",
	TrangthaiDiChuyen: "DI_CHUYEN",
	TrangthaiNhay:     "NHAY",
	TrangthaiRoi:      "ROI",
	TrangthaiBayNgang: "BAY_NGANG",
	TrangthaiThu:      "THU",
	TrangthaiGong:     "GONG",
}

var stringToTrangthai = map[string]byte{
	"DUNG_YEN":  TrangthaiDungYen,
	"DI_CHUYEN": TrangthaiDiChuyen,
	"NHAY":      TrangthaiNhay,
	"ROI":       TrangthaiRoi,
	"BAY_NGANG": TrangthaiBayNgang,
	"THU":       TrangthaiThu,
	"GONG":      TrangthaiGong,
}

func TrangthaiToString(v byte) string {
	if s, ok := trangthaiToString[v]; ok {
		return s
	}
	return "DUNG_YEN" // fallback
}

func StringToTrangthai(s string) byte {
	if v, ok := stringToTrangthai[s]; ok {
		return v
	}
	return TrangthaiDungYen
}
