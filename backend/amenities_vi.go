package main

import "strings"

// Amenities từ API (field "amenities") là chuỗi tiếng Anh. Bảng này dịch sang
// tiếng Việt để overlay/thumbnail hiển thị đúng tiếng Việt mà không cần gõ tay.
// Từ nào chưa có trong bảng thì giữ nguyên (không nuốt dữ liệu).
var amenityVI = map[string]string{
	"netflix":                    "Netflix",
	"wifi":                       "Wifi",
	"tv":                         "TV",
	"board game":                 "Board game",
	"projector":                  "Máy chiếu",
	"handheld console":           "Máy chơi game cầm tay",
	"hot water heater":           "Máy nước nóng",
	"hot water":                  "Nước nóng",
	"stand shower":               "Tắm đứng",
	"bathtub":                    "Bồn tắm",
	"hot tub":                    "Bồn tắm nước nóng",
	"jacuzzi":                    "Bồn jacuzzi",
	"bidet":                      "Vòi xịt vệ sinh",
	"shampoo":                    "Dầu gội",
	"conditional":                "Dầu xả",
	"shower gel":                 "Sữa tắm",
	"body soap":                  "Xà phòng tắm",
	"toothbrush":                 "Bàn chải",
	"toothpaste":                 "Kem đánh răng",
	"towel":                      "Khăn tắm",
	"hair dryer":                 "Máy sấy tóc",
	"refrigerator":               "Tủ lạnh",
	"mini fridge":                "Tủ lạnh mini",
	"kitchen":                    "Bếp",
	"induction cooker":           "Bếp từ",
	"rice maker":                 "Nồi cơm điện",
	"microwave":                  "Lò vi sóng",
	"coffee maker":               "Máy pha cà phê",
	"hot water kettle":           "Ấm đun nước",
	"dishwasher":                 "Máy rửa chén",
	"knife and cutboard":         "Dao và thớt",
	"cooking basics":             "Dụng cụ nấu ăn cơ bản",
	"basic seasoning":            "Gia vị cơ bản",
	"full seasoning":             "Gia vị đầy đủ",
	"dish detergent":             "Nước rửa chén",
	"dinning table":              "Bàn ăn",
	"dinning setup":              "Bộ bàn ăn",
	"dishes and silverware":      "Chén dĩa & dao nĩa",
	"dishes and spoons":          "Chén & muỗng",
	"pots and pots":              "Nồi niêu xoong chảo",
	"wineglass":                  "Ly rượu",
	"wine glasses":               "Ly rượu",
	"wine opener":                "Đồ khui rượu",
	"air conditioning":           "Máy điều hòa",
	"air conditioner with heat":  "Điều hòa 2 chiều",
	"heating":                    "Máy sưởi",
	"fan":                        "Quạt",
	"washer":                     "Máy giặt",
	"dryer":                      "Máy sấy quần áo",
	"laundry detergent":          "Nước giặt",
	"iron":                       "Bàn là",
	"hangers":                    "Móc treo quần áo",
	"full body mirror":           "Gương soi toàn thân",
	"elevator":                   "Thang máy",
	"balcony":                    "Ban công",
	"patio or balcony":           "Sân hiên / Ban công",
	"garden":                     "Sân vườn",
	"outdoor dining area":        "Khu ăn uống ngoài trời",
	"work desk":                  "Bàn làm việc",
	"laptop-friendly workspace":  "Góc làm việc cho laptop",
	"socket":                     "Ổ cắm điện",
	"extra pillows and blankets": "Chăn gối dự phòng",
	"bed linens":                 "Ga trải giường",
	"essentials":                 "Đồ dùng thiết yếu",
	"water":                      "Nước uống",
	"ice":                        "Đá viên",
	"inhouse parking space":      "Chỗ đỗ xe trong nhà",
	"parking with fee":           "Chỗ đỗ xe (có phí)",
	"private entrance":           "Lối vào riêng",
	"lock on bedroom door":       "Khóa cửa phòng ngủ",
	"smoke alarm":                "Báo khói",
	"carbon monoxide alarm":      "Báo khí CO",
	"fire extinguisher":          "Bình chữa cháy",
	"hidden camera detector":     "Máy dò camera ẩn",
	"cleaning service":           "Dịch vụ dọn phòng",
	"cleaning before checkout":   "Dọn dẹp trước khi trả phòng",
	"long term stays allowed":    "Cho thuê dài hạn",
	"party setup":                "Set up tiệc",
	"tea party":                  "Tiệc trà",
	"restaurant":                 "Nhà hàng",
	"gym":                        "Phòng gym",
	"hot water pool":             "Bể bơi nước nóng",
	"shared swimming pool":       "Bể bơi chung",
	"toaster":                    "Máy nướng bánh mì",
	"oven":                       "Lò nướng",
	"stove":                      "Bếp ga",
	"outdoor kitchen":            "Bếp ngoài trời",
	"breakfast":                  "Bữa sáng",
	"mineral_water":              "Nước khoáng",
	"first aid kit":              "Bộ sơ cứu",
	"lockbox":                    "Hộp khóa đựng chìa",
	"self check-in":              "Tự nhận phòng",
	"power generator":            "Máy phát điện",
	"clothing storage":           "Tủ quần áo",
	"drying rack for clothing":   "Giá phơi quần áo",
	"bathtub setup":              "Bồn tắm",
	"outdor furniture":           "Nội thất ngoài trời", // typo trong API
	"outdoor furniture":          "Nội thất ngoài trời",
	"photography room":           "Phòng chụp ảnh",
	"party":                      "Tiệc",
	"beach view":                 "View biển",
	"city view":                  "View thành phố",
	"lake view":                  "View hồ",
	"street view":                "View đường phố",
	"free parking moto":          "Gửi xe máy miễn phí",
	"free parking street":        "Đỗ xe đường phố miễn phí",
	"paid parking off premises":  "Đỗ xe có phí (ngoài khuôn viên)",
	"undefined":                  "", // rác → bỏ
}

// translateAmenity dịch 1 amenity tiếng Anh → tiếng Việt; chưa có trong bảng thì
// giữ nguyên (đã viết hoa chữ cái đầu cho gọn).
func translateAmenity(en string) string {
	key := strings.ToLower(strings.TrimSpace(en))
	if vi, ok := amenityVI[key]; ok {
		return vi
	}
	if en == "" {
		return ""
	}
	// Giữ nguyên từ chưa map, viết hoa chữ đầu cho đồng bộ.
	return strings.ToUpper(en[:1]) + en[1:]
}

// translateAmenities dịch cả danh sách, bỏ trùng (giữ thứ tự xuất hiện).
func translateAmenities(items []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, it := range items {
		vi := translateAmenity(it)
		if vi == "" || seen[vi] {
			continue
		}
		seen[vi] = true
		out = append(out, vi)
	}
	return out
}
