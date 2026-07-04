package main

// Chuẩn hoá dữ liệu THẬT từ API cho overlay video. App gửi cfg.Prices = price_lines
// (App.jsx) ở dạng dài/rườm rà ("Giá 2 ngày 1 đêm: Từ 519.000 đ", "Combo giờ: 10-13h:
// 260.000 đ · 10-15h: 480.000 đ · …" tới 147 ký tự) và địa chỉ có rác (",Hà Nội,Hà
// Nội,Vietnam"). 8 template video thiết kế cho dòng giá NGẮN như mockup nên data thật
// tràn khung. Hai helper dưới NÉN giá + DỌN địa chỉ về dạng video-friendly, đúng phong
// cách mockup.
//
// Quan trọng: chỉ biến đổi dòng có DẤU HIỆU rườm rà của API (Từ / 9PM-9AM / "N giờ
// đầu" / "thêm giờ" / "Combo giờ:"). Dòng đã ngắn (data mockup: "Qua đêm: 299,000",
// "Combo: 299,000", "Giá theo ngày") được GIỮ NGUYÊN → mockup không đổi.

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ─── Bảng giá kiểu mockup video (Trang 4 / chillgreen) ────────────────────────
// Mockup hiển thị giá theo KHUNG GIỜ, số rút gọn (299, 399 — không "000"): dòng
// qua đêm "21h - 9h (Qua đêm)", "2N1Đ", rồi các khung giờ lấy từ special_offer_times
// (key "1020" = 10h-20h, "1420" = 14h-20h…). Giá lấy đúng từ API thật.

// priceShort rút gọn số tiền VN về "hàng nghìn" như mockup: 278635 → "279", 319000 → "319".
func priceShort(n int64) string {
	return strconv.Itoa(int(math.Round(float64(n) / 1000.0)))
}

// jsonNumMap parse map[string]<number> từ RawMessage (bỏ qua giá trị không phải số).
func jsonNumMap(raw json.RawMessage) map[string]int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var m map[string]json.Number
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	out := make(map[string]int64, len(m))
	for k, v := range m {
		if f, err := v.Float64(); err == nil {
			out[k] = int64(math.Round(f))
		}
	}
	return out
}

// offerWindow là một khung giờ đã tách từ special_offer_times.
type offerWindow struct {
	start, end int
	price      int64
}

// specialOfferWindows đọc special_offer_times dạng {"1020":{"0":..,"1":..}, ...} →
// list khung giờ (bỏ khung qua đêm 2109, xử lý riêng). Giá lấy ngày thường (weekday
// "1" = Thứ 2); thiếu thì lấy giá thấp nhất trong tuần ("giá từ"). Sắp theo GIÁ
// tăng dần như mockup Canva (10h-13h: 269 → 14h-20h: 399 → 10h-20h: 499).
func specialOfferWindows(raw json.RawMessage) []offerWindow {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var byKey map[string]json.RawMessage
	if json.Unmarshal(raw, &byKey) != nil {
		return nil
	}
	var out []offerWindow
	for key, sub := range byKey {
		key = strings.TrimSpace(key)
		if len(key) != 4 {
			continue
		}
		s, es := strconv.Atoi(key[:2])
		e, ee := strconv.Atoi(key[2:])
		if es != nil || ee != nil {
			continue
		}
		if s == 21 && e == 9 { // qua đêm — xử lý riêng
			continue
		}
		days := jsonNumMap(sub)
		if len(days) == 0 {
			continue
		}
		price := days["1"]
		if price <= 0 {
			price = 1 << 62
			for _, v := range days {
				if v > 0 && v < price {
					price = v
				}
			}
		}
		if price <= 0 || price == 1<<62 {
			continue
		}
		out = append(out, offerWindow{start: s, end: e, price: price})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].price != out[j].price {
			return out[i].price < out[j].price
		}
		if out[i].start != out[j].start {
			return out[i].start < out[j].start
		}
		return out[i].end < out[j].end
	})
	return out
}

// overnightVideoPrice tìm giá qua đêm (21h-9h): ưu tiên combos["2109"], rồi
// special_offer_times["2109"], cuối cùng suy từ giá ngày × (1 − night_short_rate).
func overnightVideoPrice(in ListingInfo) int64 {
	if m := jsonNumMap(in.Combos); m["2109"] > 0 {
		return m["2109"]
	}
	if len(in.SpecialOffer) > 0 {
		var byKey map[string]json.RawMessage
		if json.Unmarshal(in.SpecialOffer, &byKey) == nil {
			if days := jsonNumMap(byKey["2109"]); days["1"] > 0 {
				return days["1"]
			}
		}
	}
	if base := priceByWeekday(in.PricesByWeek, 1); base > 0 {
		rate := in.NightShortRate
		if rate <= 0 {
			rate = 0.3
		}
		return int64(math.Round((1 - rate) * float64(base)))
	}
	return 0
}

// mockupPriceLines dựng các dòng giá kiểu mockup video (chillgreen) từ data API thật.
// Thứ tự: qua đêm, 2N1Đ (nhóm 1) → các khung giờ (nhóm 2), khớp bố cục 2 khối của template.
func mockupPriceLines(in ListingInfo) []string {
	var out []string
	if n := overnightVideoPrice(in); n > 0 {
		out = append(out, "21h - 9h (Qua đêm): "+priceShort(n))
	}
	if n := priceByWeekday(in.PricesByWeek, 1); n > 0 {
		out = append(out, "2N1Đ: "+priceShort(n))
	}
	for _, w := range specialOfferWindows(in.SpecialOffer) {
		out = append(out, fmt.Sprintf("%dh - %dh: %s", w.start, w.end, priceShort(w.price)))
	}
	return out
}

// ─── Bảng giá RIÊNG cho từng template (khớp y hệt nhãn + đơn vị của mockup Canva) ──
// Mỗi trang Canva có bộ nhãn + đơn vị số khác nhau:
//   goldserif  (T1): nhóm "Giá theo ngày/giờ", số đầy đủ dấu phẩy — "2N1Đ: 390,000"
//   creampill  (T2): "Qua đêm: … - Combo: …" số đầy đủ (template tự nối " - ")
//   staycation (T3): "2N1Đ / Qua đêm / combo theo giờ", số đầy đủ
//   chillgreen (T4) + marquee (T5): khung giờ, số rút gọn + 🐟 (template tự thêm)
//   editorial  (T6) + amorex (T7): "Giá giờ / Giá qua đêm / 2N1Đ", đơn vị " cá"
//   ntgroom    (T8): "2N1Đ / Qua đêm / Thêm giờ / Nh đầu / Combo giờ", đơn vị "k"

// commaVND: số đầy đủ, LÀM TRÒN NGHÌN, phân tách bằng dấu phẩy như mockup
// ("278.635" → "279,000"). Giá lẻ do promo tính ra được làm gọn về nghìn tròn.
func commaVND(n int64) string {
	rounded := int64(math.Round(float64(n)/1000.0)) * 1000
	return strings.ReplaceAll(formatVND(rounded), ".", ",")
}
func caUnit(n int64) string { return priceShort(n) + " cá" }
func kUnit(n int64) string  { return priceShort(n) + "k" }

// comboHourPrice = giá combo giờ (deal giờ rẻ nhất): min các khung special_offer_times
// (bỏ qua đêm), thiếu thì min combos (bỏ "2109").
func comboHourPrice(in ListingInfo) int64 {
	const big = int64(1) << 62
	min := big
	for _, w := range specialOfferWindows(in.SpecialOffer) {
		if w.price > 0 && w.price < min {
			min = w.price
		}
	}
	if min == big {
		for k, v := range jsonNumMap(in.Combos) {
			if k == "2109" {
				continue
			}
			if v > 0 && v < min {
				min = v
			}
		}
	}
	if min == big {
		return 0
	}
	return min
}

// templatePriceLines dựng đúng bộ dòng giá cho 1 template từ data API thật.
func templatePriceLines(in ListingInfo, template string) []string {
	twoNight := priceByWeekday(in.PricesByWeek, 1)
	overnight := overnightVideoPrice(in)
	firstP := in.PriceFirstHours
	firstN := in.FirstHours
	if firstN <= 0 {
		firstN = 2
	}
	extra := in.PricePerHour
	combo := comboHourPrice(in)
	windows := specialOfferWindows(in.SpecialOffer)

	var out []string
	add := func(cond bool, s string) {
		if cond {
			out = append(out, s)
		}
	}
	switch strings.ToLower(strings.TrimSpace(template)) {
	case "chillgreen", "marquee":
		return mockupPriceLines(in)

	case "amorex", "editorial":
		add(firstP > 0, "Giá giờ: "+caUnit(firstP))
		add(overnight > 0, "Giá qua đêm: "+caUnit(overnight))
		add(twoNight > 0, "2N1Đ: "+caUnit(twoNight))

	case "ntgroom":
		add(twoNight > 0, "2N1Đ: "+kUnit(twoNight))
		add(overnight > 0, "Qua đêm: "+kUnit(overnight))
		add(extra > 0, "Thêm giờ: "+kUnit(extra))
		add(firstP > 0, fmt.Sprintf("%dh đầu: %s", firstN, kUnit(firstP)))
		add(combo > 0, "Combo giờ: "+kUnit(combo))

	case "creampill":
		add(overnight > 0, "Qua đêm: "+commaVND(overnight))
		add(combo > 0, "Combo: "+commaVND(combo))

	case "staycation":
		add(twoNight > 0, "2N1Đ: "+commaVND(twoNight))
		add(overnight > 0, "Qua đêm: "+commaVND(overnight))
		add(combo > 0, "combo theo giờ: "+commaVND(combo))

	case "goldserif":
		if twoNight > 0 || overnight > 0 {
			out = append(out, "Giá theo ngày")
			add(twoNight > 0, "2N1Đ: "+commaVND(twoNight))
			add(overnight > 0, "Qua đêm: "+commaVND(overnight))
		}
		if len(windows) > 0 {
			out = append(out, "Giá theo giờ")
			for _, w := range windows {
				out = append(out, fmt.Sprintf("%dh - %dh: %s", w.start, w.end, commaVND(w.price)))
			}
		}

	default:
		return nil
	}
	return out
}

// allTemplateNames — 8 template video có bảng giá riêng.
var allTemplateNames = []string{"goldserif", "creampill", "staycation", "chillgreen", "marquee", "editorial", "amorex", "ntgroom"}

// priceLinesByTemplate dựng map template → dòng giá cho toàn bộ 8 template.
func priceLinesByTemplate(in ListingInfo) map[string][]string {
	m := map[string][]string{}
	for _, tpl := range allTemplateNames {
		if lines := templatePriceLines(in, tpl); len(lines) > 0 {
			m[tpl] = lines
		}
	}
	return m
}

// rePriceAmt khớp số tiền VN có nhóm nghìn: "519.000", "467.100", "1.500.000".
// Không khớp "9PM-9AM", "10-13h", "2 ngày" (không có nhóm .ddd).
var rePriceAmt = regexp.MustCompile(`\d{1,3}(?:\.\d{3})+`)
var reFirstHours = regexp.MustCompile(`(\d+)\s*giờ\s*đầu`)
var reTrailingPostal = regexp.MustCompile(`\s*\d{4,}\s*$`)

func kInt(n int) string { return strconv.Itoa((n+500)/1000) + "k" }

func amtToK(s string) string {
	n, err := strconv.Atoi(strings.ReplaceAll(s, ".", ""))
	if err != nil {
		return s
	}
	return kInt(n)
}

// compactVideoPriceLine nén MỘT dòng giá nếu nó là dòng rườm rà của API; nếu không
// (đã ngắn) trả nguyên văn.
func compactVideoPriceLine(line string) string {
	l := strings.TrimSpace(line)
	if l == "" {
		return l
	}
	low := strings.ToLower(l)
	amts := rePriceAmt.FindAllString(l, -1)

	// Combo nhiều khung "Combo giờ: 10-13h: … · 10-15h: …" → "Combo: từ {rẻ nhất}".
	if strings.HasPrefix(low, "combo giờ") {
		if len(amts) > 0 {
			min := 1 << 60
			for _, a := range amts {
				if n, err := strconv.Atoi(strings.ReplaceAll(a, ".", "")); err == nil && n < min {
					min = n
				}
			}
			return "Combo: từ " + kInt(min)
		}
		return l
	}

	// Chỉ nén khi có dấu hiệu rườm rà của API (tránh đụng data mockup đã ngắn).
	verbose := strings.Contains(l, "Từ") ||
		strings.Contains(low, "9pm-9am") ||
		reFirstHours.MatchString(low) ||
		strings.Contains(low, "thêm giờ")
	if !verbose || len(amts) == 0 {
		return l
	}
	amt := amtToK(amts[0])
	switch {
	case strings.Contains(low, "ngày") && strings.Contains(low, "đêm"):
		return "2N1Đ: " + amt
	case strings.Contains(low, "qua đêm"):
		return "Qua đêm: " + amt
	case reFirstHours.MatchString(low):
		h := reFirstHours.FindStringSubmatch(low)[1]
		return h + "h đầu: " + amt
	case strings.Contains(low, "thêm giờ"):
		return "Thêm giờ: " + amt
	}
	return l
}

// compactVideoPrices áp compactVideoPriceLine cho từng dòng-vật-lý (mỗi phần tử có
// thể chứa "\n"), giữ nguyên cấu trúc nhóm.
func compactVideoPrices(prices []string) []string {
	if len(prices) == 0 {
		return prices
	}
	out := make([]string, 0, len(prices))
	for _, p := range prices {
		lines := strings.Split(p, "\n")
		for i, ln := range lines {
			lines[i] = compactVideoPriceLine(ln)
		}
		out = append(out, strings.Join(lines, "\n"))
	}
	return out
}

// pickTwoPriceLines chọn tối đa 2 dòng giá tiêu biểu cho template chỉ có chỗ cho 2
// (creampill). Ưu tiên "Qua đêm" + "Combo"; thiếu thì lấp bằng các dòng đầu. Nếu
// đầu vào ≤2 dòng → giữ nguyên (data mockup không đổi).
func pickTwoPriceLines(lines []string) []string {
	if len(lines) <= 2 {
		return lines
	}
	var overnight, combo string
	for _, l := range lines {
		low := strings.ToLower(l)
		if overnight == "" && strings.Contains(low, "qua đêm") {
			overnight = l
		}
		if combo == "" && strings.HasPrefix(low, "combo") {
			combo = l
		}
	}
	var out []string
	if overnight != "" {
		out = append(out, overnight)
	}
	if combo != "" {
		out = append(out, combo)
	}
	for _, l := range lines { // lấp cho đủ 2 nếu thiếu
		if len(out) >= 2 {
			break
		}
		dup := false
		for _, o := range out {
			if o == l {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, l)
		}
	}
	return out
}

// goldSerifGroups gom dòng giá thật thành 2 nhóm có HEADER vàng "Giá theo ngày" /
// "Giá theo giờ" (goldserif dùng dòng không-chứa-số làm header). Nếu data đã có
// header sẵn (mockup) hoặc rỗng → giữ nguyên.
func goldSerifGroups(lines []string) []string {
	var flat []string
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		flat = append(flat, l)
		if !strings.ContainsAny(l, "0123456789") {
			return lines // đã có header → mockup, giữ nguyên
		}
	}
	if len(flat) == 0 {
		return lines
	}
	var day, hour []string
	for _, l := range flat {
		low := strings.ToLower(l)
		if strings.Contains(low, "n1đ") || strings.Contains(low, "qua đêm") || strings.Contains(low, "ngày") {
			day = append(day, l)
		} else {
			hour = append(hour, l)
		}
	}
	var out []string
	if len(day) > 0 {
		out = append(out, "Giá theo ngày")
		out = append(out, day...)
	}
	if len(hour) > 0 {
		out = append(out, "Giá theo giờ")
		out = append(out, hour...)
	}
	return out
}

// districtProvinceLine rút "Quận - Tỉnh" từ địa chỉ đầy đủ cho dòng địa danh
// template chillgreen ("...,Thanh Xuân,Hà Nội,Hà Nội,Vietnam" → "Thanh Xuân - Hà Nội").
// Bỏ phố (phần đầu) + Vietnam/lặp, lấy 2 thành phần cuối làm quận + tỉnh.
func districtProvinceLine(addr string) string {
	clean := cleanVideoAddress(addr)
	if clean == "" {
		return ""
	}
	var parts []string
	for _, p := range strings.Split(clean, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return parts[len(parts)-2] + " - " + parts[len(parts)-1]
	}
}

// cleanVideoAddress bỏ "Vietnam"/quốc gia, mã bưu chính đuôi, và phần lặp ("Hà
// Nội,Hà Nội") → địa chỉ gọn 3–4 thành phần như mockup.
func cleanVideoAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return addr
	}
	parts := strings.Split(addr, ",")
	var out []string
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = strings.TrimSpace(reTrailingPostal.ReplaceAllString(p, ""))
		if p == "" {
			continue
		}
		lp := strings.ToLower(p)
		if lp == "vietnam" || lp == "việt nam" || lp == "viet nam" {
			continue
		}
		if seen[lp] {
			continue
		}
		seen[lp] = true
		out = append(out, p)
	}
	return strings.Join(out, ", ")
}
