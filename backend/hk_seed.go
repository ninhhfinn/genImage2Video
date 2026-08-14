package main

// Mẫu checklist khởi tạo.
//
// Chỉ chạy khi DB chưa có mẫu nào — quản lý sửa xong thì lần khởi động sau không
// bị ghi đè. Đây là điểm bắt đầu để sửa, không phải quy chuẩn bất biến: mỗi nhà
// một kiểu, và người biết rõ phải dọn gì là quản lý vận hành chứ không phải tôi.

import "strings"

func hkItem(id, title string, requirePhoto bool, minPhotos int, hint string) HKItem {
	return HKItem{ID: id, Title: title, RequirePhoto: requirePhoto, MinPhotos: minPhotos, Hint: hint}
}

func hkGroupBedroom(suffix, title string) HKGroup {
	id := func(base string) string { return base + suffix }
	return HKGroup{
		ID:    id("g_bedroom"),
		Title: title,
		Items: []HKItem{
			hkItem(id("i_bed_linen"), "Thay ga, vỏ gối, vỏ chăn mới", true, 1, "Chụp giường đã trải xong, thấy cả 4 góc"),
			hkItem(id("i_bed_vacuum"), "Hút bụi / lau sàn phòng ngủ", true, 1, ""),
			hkItem(id("i_bed_under"), "Kiểm tra gầm giường, tủ đầu giường", true, 1, "Khách hay bỏ quên đồ ở đây"),
			hkItem(id("i_bed_aircon"), "Lau mặt nạ điều hoà, bật thử", false, 0, ""),
		},
	}
}

func hkGroupBath() HKGroup {
	return HKGroup{
		ID:    "g_bath",
		Title: "Phòng tắm",
		Items: []HKItem{
			hkItem("i_bath_toilet", "Cọ bồn cầu", true, 1, ""),
			hkItem("i_bath_floor", "Cọ sàn, kính, vòi sen", true, 1, ""),
			hkItem("i_bath_towel", "Bổ sung khăn tắm, khăn mặt", true, 1, "Chụp rõ số khăn đã treo"),
			hkItem("i_bath_amenity", "Bổ sung dầu gội, sữa tắm, giấy vệ sinh", true, 1, ""),
		},
	}
}

func hkGroupKitchen() HKGroup {
	return HKGroup{
		ID:    "g_kitchen",
		Title: "Bếp",
		Items: []HKItem{
			hkItem("i_kit_dish", "Rửa và úp gọn bát đĩa, cốc chén", true, 1, ""),
			hkItem("i_kit_counter", "Lau mặt bếp, bồn rửa, ấm siêu tốc", true, 1, ""),
			hkItem("i_kit_fridge", "Dọn tủ lạnh, bỏ đồ khách để lại", true, 1, "Chụp bên trong tủ lạnh sau khi dọn"),
		},
	}
}

func hkGroupLiving() HKGroup {
	return HKGroup{
		ID:    "g_living",
		Title: "Khu vực chung",
		Items: []HKItem{
			hkItem("i_liv_floor", "Lau sàn phòng khách, lối vào", true, 1, ""),
			hkItem("i_liv_sofa", "Sắp xếp sofa, gối tựa, rèm", true, 1, ""),
			hkItem("i_liv_trash", "Đổ rác toàn căn, thay túi rác mới", true, 1, ""),
		},
	}
}

func hkGroupStairs() HKGroup {
	return HKGroup{
		ID:    "g_stairs",
		Title: "Cầu thang & tầng trên",
		Items: []HKItem{
			hkItem("i_st_stairs", "Lau cầu thang, tay vịn", true, 1, ""),
			hkItem("i_st_upper", "Dọn phòng tầng trên", true, 1, ""),
		},
	}
}

func hkGroupFinal() HKGroup {
	return HKGroup{
		ID:    "g_final",
		Title: "Kiểm tra cuối",
		Items: []HKItem{
			hkItem("i_fin_overview", "Ảnh tổng thể căn sau khi xong", true, 2,
				"Chụp 2 góc đối diện nhau để thấy toàn bộ căn"),
			hkItem("i_fin_lock", "Tắt điều hoà, đèn, khoá cửa", true, 1, ""),
			hkItem("i_fin_report", "Báo hỏng hóc / thiếu đồ cho chủ nhà (nếu có)", false, 0,
				"Không có gì hỏng thì tick qua"),
		},
	}
}

func hkDefaultTemplates(now int64) []HKTemplate {
	return []HKTemplate{
		{
			ID:        "hkt_studio",
			Name:      "Chuẩn Studio / 1 phòng ngủ",
			RoomTypes: []string{"studio", "one_bedroom"},
			UpdatedAt: now,
			Groups: []HKGroup{
				hkGroupBedroom("", "Phòng ngủ"),
				hkGroupBath(),
				hkGroupKitchen(),
				hkGroupLiving(),
				hkGroupFinal(),
			},
		},
		{
			ID:        "hkt_2pn",
			Name:      "2 phòng ngủ",
			RoomTypes: []string{"two_bedroom"},
			UpdatedAt: now,
			Groups: []HKGroup{
				hkGroupBedroom("", "Phòng ngủ 1"),
				hkGroupBedroom("_2", "Phòng ngủ 2"),
				hkGroupBath(),
				hkGroupKitchen(),
				hkGroupLiving(),
				hkGroupFinal(),
			},
		},
		{
			ID:        "hkt_duplex",
			Name:      "Duplex / nhiều tầng",
			RoomTypes: []string{"duplex"},
			UpdatedAt: now,
			Groups: []HKGroup{
				hkGroupBedroom("", "Phòng ngủ"),
				hkGroupStairs(),
				hkGroupBath(),
				hkGroupKitchen(),
				hkGroupLiving(),
				hkGroupFinal(),
			},
		},
	}
}

func (a *HKApp) hkSeedTemplates() error {
	existing, err := a.store.ListTemplates()
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	for _, t := range hkDefaultTemplates(hkNowMs()) {
		if err := a.store.UpsertTemplate(t); err != nil {
			return err
		}
	}
	return nil
}

// hkTotalRequiredPhotos đếm số ảnh tối thiểu một ca đòi hỏi — dùng để cảnh báo
// khi quản lý dựng mẫu quá nặng. Trên ~20 ảnh mỗi phòng là bắt đầu có người bỏ cuộc.
func hkTotalRequiredPhotos(t *HKTemplate) int {
	total := 0
	for _, it := range hkFlattenItems(t) {
		if it.RequirePhoto {
			total += hkMinPhotos(it)
		}
	}
	return total
}

func hkTemplateSummary(t *HKTemplate) string {
	if t == nil {
		return ""
	}
	var titles []string
	for _, g := range t.Groups {
		titles = append(titles, g.Title)
	}
	return strings.Join(titles, " · ")
}
