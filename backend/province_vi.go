package main

import "strings"

// ─── Chuẩn hóa TÊN TỈNH/THÀNH có dấu ─────────────────────────────────────────
//
// Địa chỉ listing đôi khi lưu KHÔNG dấu ("Hung Yen") → badge thumbnail ra "HUNG
// YEN". canonicalProvince bỏ dấu để so khớp rồi trả lại tên có dấu chuẩn từ bảng
// 63 tỉnh/thành. Không khớp → giữ nguyên chuỗi gốc.

// viStripLower hạ chữ thường + bỏ dấu tiếng Việt (chỉ cần cho việc so khớp key).
var viStripLower = strings.NewReplacer(
	"à", "a", "á", "a", "ả", "a", "ã", "a", "ạ", "a",
	"ă", "a", "ằ", "a", "ắ", "a", "ẳ", "a", "ẵ", "a", "ặ", "a",
	"â", "a", "ầ", "a", "ấ", "a", "ẩ", "a", "ẫ", "a", "ậ", "a",
	"è", "e", "é", "e", "ẻ", "e", "ẽ", "e", "ẹ", "e",
	"ê", "e", "ề", "e", "ế", "e", "ể", "e", "ễ", "e", "ệ", "e",
	"ì", "i", "í", "i", "ỉ", "i", "ĩ", "i", "ị", "i",
	"ò", "o", "ó", "o", "ỏ", "o", "õ", "o", "ọ", "o",
	"ô", "o", "ồ", "o", "ố", "o", "ổ", "o", "ỗ", "o", "ộ", "o",
	"ơ", "o", "ờ", "o", "ớ", "o", "ở", "o", "ỡ", "o", "ợ", "o",
	"ù", "u", "ú", "u", "ủ", "u", "ũ", "u", "ụ", "u",
	"ư", "u", "ừ", "u", "ứ", "u", "ử", "u", "ữ", "u", "ự", "u",
	"ỳ", "y", "ý", "y", "ỷ", "y", "ỹ", "y", "ỵ", "y",
	"đ", "d",
)

func viStrip(s string) string { return viStripLower.Replace(strings.ToLower(s)) }

// provinceCanon: key = tên đã bỏ dấu/thường (đã bỏ tiền tố tỉnh/tp) → tên có dấu.
var provinceCanon = map[string]string{
	"an giang": "An Giang", "ba ria - vung tau": "Bà Rịa - Vũng Tàu",
	"ba ria vung tau": "Bà Rịa - Vũng Tàu", "vung tau": "Vũng Tàu",
	"bac giang": "Bắc Giang", "bac kan": "Bắc Kạn", "bac lieu": "Bạc Liêu",
	"bac ninh": "Bắc Ninh", "ben tre": "Bến Tre", "binh dinh": "Bình Định",
	"binh duong": "Bình Dương", "binh phuoc": "Bình Phước", "binh thuan": "Bình Thuận",
	"ca mau": "Cà Mau", "can tho": "Cần Thơ", "cao bang": "Cao Bằng",
	"da nang": "Đà Nẵng", "dak lak": "Đắk Lắk", "dak nong": "Đắk Nông",
	"dien bien": "Điện Biên", "dong nai": "Đồng Nai", "dong thap": "Đồng Tháp",
	"gia lai": "Gia Lai", "ha giang": "Hà Giang", "ha nam": "Hà Nam",
	"ha noi": "Hà Nội", "ha tinh": "Hà Tĩnh", "hai duong": "Hải Dương",
	"hai phong": "Hải Phòng", "hau giang": "Hậu Giang", "hoa binh": "Hòa Bình",
	"hung yen": "Hưng Yên", "khanh hoa": "Khánh Hòa", "nha trang": "Nha Trang",
	"kien giang": "Kiên Giang", "kon tum": "Kon Tum", "lai chau": "Lai Châu",
	"lam dong": "Lâm Đồng", "da lat": "Đà Lạt", "lang son": "Lạng Sơn",
	"lao cai": "Lào Cai", "sa pa": "Sa Pa", "sapa": "Sa Pa", "long an": "Long An",
	"nam dinh": "Nam Định", "nghe an": "Nghệ An", "ninh binh": "Ninh Bình",
	"ninh thuan": "Ninh Thuận", "phu tho": "Phú Thọ", "phu yen": "Phú Yên",
	"quang binh": "Quảng Bình", "quang nam": "Quảng Nam", "hoi an": "Hội An",
	"quang ngai": "Quảng Ngãi", "quang ninh": "Quảng Ninh", "ha long": "Hạ Long",
	"quang tri": "Quảng Trị", "soc trang": "Sóc Trăng", "son la": "Sơn La",
	"tay ninh": "Tây Ninh", "thai binh": "Thái Bình", "thai nguyen": "Thái Nguyên",
	"thanh hoa": "Thanh Hóa", "thua thien hue": "Thừa Thiên Huế", "hue": "Huế",
	"tien giang": "Tiền Giang", "tra vinh": "Trà Vinh", "tuyen quang": "Tuyên Quang",
	"vinh long": "Vĩnh Long", "vinh phuc": "Vĩnh Phúc", "yen bai": "Yên Bái",
	"ho chi minh": "Hồ Chí Minh", "sai gon": "Sài Gòn", "tphcm": "Hồ Chí Minh",
	"hcm": "Hồ Chí Minh",
}

// canonicalProvince trả tên tỉnh có dấu chuẩn; không khớp bảng → giữ nguyên gốc.
func canonicalProvince(name string) string {
	key := strings.TrimSpace(viStrip(name))
	for _, p := range []string{"tinh ", "thanh pho ", "tp. ", "tp ", "tp.", "tp"} {
		if strings.HasPrefix(key, p) {
			key = strings.TrimSpace(strings.TrimPrefix(key, p))
			break
		}
	}
	if v, ok := provinceCanon[key]; ok {
		return v
	}
	return strings.TrimSpace(name)
}
