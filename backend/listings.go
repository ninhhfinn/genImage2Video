package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─── Data types ───────────────────────────────────────────────────────────

type ListingInfo struct {
	Index           int             `json:"index"`
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Nickname        string          `json:"nickname"`
	Address         string          `json:"address"`
	Price           int64           `json:"price"`
	Beds            int             `json:"beds"`
	PhotoURLs       []string        `json:"photo_urls"`
	ThumbURLs       []string        `json:"thumb_urls"`
	PhotoCount      int             `json:"photo_count"`
	PriceFirstHours int64           `json:"price_first_hours"`
	PricePerHour    int64           `json:"price_per_hour"`
	FirstHours      int             `json:"first_hours"`
	TotalPrice      int64           `json:"total_price"`
	PricesByWeek    []int64         `json:"prices_by_week"`
	NightShortRate  float64         `json:"night_short_rate"`
	SpecialOffer    json.RawMessage `json:"special_offer_times,omitempty"`
	Combos          json.RawMessage `json:"combos,omitempty"`
	PriceLines      []string        `json:"price_lines"`       // các dòng giá đã format sẵn
	VideoPriceLines []string        `json:"video_price_lines"` // giá kiểu mockup video (khung giờ) cho overlay (chillgreen)
	// PriceLinesByTemplate: bảng giá RIÊNG cho từng template video (nhãn + đơn vị y hệt mockup Canva).
	PriceLinesByTemplate map[string][]string `json:"price_lines_by_template"`
	PriceLineItems  []PriceLineItem `json:"price_line_items"`  // các dòng giá có key để UI lọc
	Amenities       []string        `json:"amenities"`        // tiện nghi (nếu API trả về)
}

type PriceLineItem struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

type rawPic struct {
	PictureURL string `json:"picture_url"`
	Picture    struct {
		URL          string `json:"url"`
		ThumbnailURL string `json:"thumbnail_url"`
		Thumbnail720 string `json:"thumbnail_720"`
	} `json:"picture"`
}

type rawListing struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Nickname        string          `json:"nickname"`
	Address         string          `json:"address"`
	FullAddress     string          `json:"full_address"`
	Price           json.RawMessage `json:"price"`
	Beds            json.RawMessage `json:"beds"`
	Pictures        []rawPic        `json:"pictures"`
	PhotoURLs       []string        `json:"photo_urls"`
	ThumbURLs       []string        `json:"thumb_urls"`
	PictureURLs     string          `json:"picture_urls"`
	ThumbnailURLs   string          `json:"thumbnail_urls"`
	PriceFirstHours json.RawMessage `json:"price_first_hours"`
	PricePerHour    json.RawMessage `json:"price_per_hour"`
	FirstHours      json.RawMessage `json:"first_hours"`
	TotalPrice      json.RawMessage `json:"total_price"`
	PricesByWeek    json.RawMessage `json:"prices_by_week"`
	PricesPerWeek   json.RawMessage `json:"prices_per_week"`
	NightShortRate  json.RawMessage `json:"night_short_rate"`
	SpecialOffer    json.RawMessage `json:"special_offer_times"`
	Combos          json.RawMessage `json:"combos"`
	Amenities       json.RawMessage `json:"amenities"`
	Facilities      json.RawMessage `json:"facilities"`
	Features        json.RawMessage `json:"features"`
}

// ─── Parse listings from raw JSON ─────────────────────────────────────────

func parseListings(data []byte) []ListingInfo {
	// 1. {"listings":[...]}
	var wrapper struct {
		Listings []rawListing `json:"listings"`
	}
	if json.Unmarshal(data, &wrapper) == nil && len(wrapper.Listings) > 0 {
		return toInfoSlice(wrapper.Listings)
	}

	// 2. [...] mảng trực tiếp
	var arr []rawListing
	if json.Unmarshal(data, &arr) == nil && len(arr) > 0 {
		return toInfoSlice(arr)
	}

	// 3. Listing đơn {}
	var single rawListing
	if json.Unmarshal(data, &single) == nil {
		photoURLs, _ := listingPhotoURLs(single)
		if len(photoURLs) > 0 {
			return toInfoSlice([]rawListing{single})
		}
	}

	if nested := findRawListings(data); len(nested) > 0 {
		if infos := toInfoSlice(nested); len(infos) > 0 {
			return infos
		}
	}

	return nil
}

func findRawListings(raw json.RawMessage) []rawListing {
	var arr []rawListing
	if json.Unmarshal(raw, &arr) == nil {
		for _, item := range arr {
			photoURLs, _ := listingPhotoURLs(item)
			if len(photoURLs) > 0 {
				return arr
			}
		}
	}

	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}

	for _, key := range []string{"listings", "items", "results", "records", "data"} {
		if child, ok := obj[key]; ok {
			if found := findRawListings(child); len(found) > 0 {
				return found
			}
		}
	}
	for _, child := range obj {
		if found := findRawListings(child); len(found) > 0 {
			return found
		}
	}

	return nil
}

func toInfoSlice(raws []rawListing) []ListingInfo {
	out := make([]ListingInfo, 0, len(raws))
	for i, r := range raws {
		photoURLs, thumbURLs := listingPhotoURLs(r)
		if len(photoURLs) == 0 {
			continue
		}
		price := jsonInt(r.Price)
		priceFirstHours := jsonInt(r.PriceFirstHours)
		pricePerHour := jsonInt(r.PricePerHour)
		firstHours := int(jsonInt(r.FirstHours))
		totalPrice := jsonInt(r.TotalPrice)
		pricesByWeek := jsonIntSlice(r.PricesByWeek)
		if len(pricesByWeek) == 0 {
			pricesByWeek = jsonIntSlice(r.PricesPerWeek)
		}
		nightShortRate := jsonFloat(r.NightShortRate)
		address := strings.TrimSpace(r.Address)
		if address == "" {
			address = strings.TrimSpace(r.FullAddress)
		}
		info := ListingInfo{
			Index:           i,
			ID:              r.ID,
			Name:            r.Name,
			Nickname:        r.Nickname,
			Address:         address,
			Price:           price,
			Beds:            int(jsonInt(r.Beds)),
			PhotoCount:      len(photoURLs),
			PriceFirstHours: priceFirstHours,
			PricePerHour:    pricePerHour,
			FirstHours:      firstHours,
			TotalPrice:      totalPrice,
			PricesByWeek:    pricesByWeek,
			NightShortRate:  nightShortRate,
			SpecialOffer:    r.SpecialOffer,
			Combos:          r.Combos,
		}
		// Build price lines tự động
		var items []PriceLineItem
		twoDayOneNightPrice := priceByWeekday(pricesByWeek, 1)
		if twoDayOneNightPrice > 0 {
			items = append(items, PriceLineItem{
				Key:  "two_day_one_night",
				Text: fmt.Sprintf("Giá 2 ngày 1 đêm: Từ %s đ", formatVND(twoDayOneNightPrice)),
			})
		}
		mondayPrice := priceByWeekday(pricesByWeek, 1)
		if mondayPrice > 0 {
			// Giá qua đêm 9PM-9AM = giá ngày thường SAU KHI giảm night_short_rate.
			// night_short_rate là TỈ LỆ GIẢM (vd 0.3 = giảm 30% → khách trả 70%),
			// KHÔNG phải hệ số nhân. Đã kiểm chứng khớp special_offer_times của API
			// (vd giá ngày 399k, rate 0.3 → qua đêm 279k, không phải 119k).
			rate := nightShortRate
			if rate <= 0 {
				rate = 0.3 // fallback khi API không trả night_short_rate
			}
			nightPrice := int64(math.Round((1 - rate) * float64(mondayPrice)))
			if nightPrice > 0 {
				items = append(items, PriceLineItem{
					Key:  "overnight",
					Text: fmt.Sprintf("Qua đêm 9PM-9AM: Từ %s đ", formatVND(nightPrice)),
				})
			}
		}
		if priceFirstHours > 0 {
			// price_first_hours là giá cho first_hours giờ đầu (vd 4 → "Giá 4 giờ đầu").
			hourLabel := "Giá giờ đầu"
			if firstHours > 0 {
				hourLabel = fmt.Sprintf("Giá %d giờ đầu", firstHours)
			}
			items = append(items, PriceLineItem{
				Key:  "hourly",
				Text: fmt.Sprintf("%s: %s đ", hourLabel, formatVND(priceFirstHours)),
			})
		}
		if pricePerHour > 0 {
			items = append(items, PriceLineItem{
				Key:  "extra_hour",
				Text: fmt.Sprintf("Giá thêm giờ: %s đ", formatVND(pricePerHour)),
			})
		}
		if combo := specialOfferForWeekday(r.SpecialOffer, 1); combo != "" {
			items = append(items, PriceLineItem{
				Key:  "hour_combo",
				Text: "Combo giờ: " + combo,
			})
		}
		info.PriceLineItems = items
		for _, item := range items {
			info.PriceLines = append(info.PriceLines, item.Text)
		}
		info.VideoPriceLines = mockupPriceLines(info)
		info.PriceLinesByTemplate = priceLinesByTemplate(info)

		// Amenities API là tiếng Anh → dịch sang tiếng Việt để hiển thị đúng.
		info.Amenities = translateAmenities(firstNonEmptyStrings(r.Amenities, r.Facilities, r.Features))
		info.PhotoURLs = photoURLs
		info.ThumbURLs = thumbURLs
		out = append(out, info)
	}
	return out
}

func listingPhotoURLs(r rawListing) ([]string, []string) {
	var photos, thumbs []string
	for _, p := range r.Pictures {
		full := p.Picture.URL
		if full == "" {
			full = p.PictureURL
		}
		thumb := p.Picture.Thumbnail720
		if thumb == "" {
			thumb = p.Picture.ThumbnailURL
		}
		if thumb == "" {
			thumb = full
		}
		if full != "" {
			photos = append(photos, full)
			thumbs = append(thumbs, thumb)
		}
	}
	pictureURLs := splitURLList(r.PictureURLs)
	thumbnailURLs := splitURLList(r.ThumbnailURLs)
	for i, full := range pictureURLs {
		thumb := full
		if i < len(thumbnailURLs) && thumbnailURLs[i] != "" {
			thumb = thumbnailURLs[i]
		}
		photos = append(photos, full)
		thumbs = append(thumbs, thumb)
	}
	for i, full := range r.PhotoURLs {
		if full == "" {
			continue
		}
		thumb := full
		if i < len(r.ThumbURLs) && r.ThumbURLs[i] != "" {
			thumb = r.ThumbURLs[i]
		}
		photos = append(photos, full)
		thumbs = append(thumbs, thumb)
	}
	seen := map[string]bool{}
	uniqPhotos := make([]string, 0, len(photos))
	uniqThumbs := make([]string, 0, len(thumbs))
	for i, photo := range photos {
		if seen[photo] {
			continue
		}
		seen[photo] = true
		thumb := photo
		if i < len(thumbs) && thumbs[i] != "" {
			thumb = thumbs[i]
		}
		uniqPhotos = append(uniqPhotos, photo)
		uniqThumbs = append(uniqThumbs, thumb)
	}
	return uniqPhotos, uniqThumbs
}

func splitURLList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func priceByWeekday(prices []int64, weekdayIdx int) int64 {
	if weekdayIdx < 0 || weekdayIdx >= len(prices) {
		return 0
	}
	return prices[weekdayIdx]
}

func jsonInt(raw json.RawMessage) int64 {
	return int64(math.Round(jsonFloat(raw)))
}

func jsonFloat(raw json.RawMessage) float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}

	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}

	var s string
	if json.Unmarshal(raw, &s) == nil {
		return parseNumberString(s)
	}

	return 0
}

func jsonIntSlice(raw json.RawMessage) []int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		out := make([]int64, len(arr))
		for i, item := range arr {
			out[i] = jsonInt(item)
		}
		return out
	}

	var s string
	if json.Unmarshal(raw, &s) == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "{") {
			return jsonIntSlice(json.RawMessage(s))
		}
		values := strings.FieldsFunc(s, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r'
		})
		out := make([]int64, 0, len(values))
		for _, value := range values {
			if n := parseNumberString(value); n > 0 {
				out = append(out, int64(math.Round(n)))
			}
		}
		return out
	}

	var byDay map[string]json.RawMessage
	if json.Unmarshal(raw, &byDay) == nil {
		out := make([]int64, 7)
		dayKeys := map[string]int{
			"0": 0, "sun": 0, "sunday": 0, "chủ nhật": 0,
			"1": 1, "2": 1, "mon": 1, "monday": 1, "thu_2": 1, "thứ 2": 1,
			"3": 2, "tue": 2, "tuesday": 2, "thu_3": 2, "thứ 3": 2,
			"4": 3, "wed": 3, "wednesday": 3, "thu_4": 3, "thứ 4": 3,
			"5": 4, "thu": 4, "thursday": 4, "thu_5": 4, "thứ 5": 4,
			"6": 5, "fri": 5, "friday": 5, "thu_6": 5, "thứ 6": 5,
			"7": 6, "sat": 6, "saturday": 6, "thu_7": 6, "thứ 7": 6,
		}
		for key, value := range byDay {
			if idx, ok := dayKeys[strings.ToLower(strings.TrimSpace(key))]; ok {
				out[idx] = jsonInt(value)
			}
		}
		return out
	}

	return nil
}

func parseNumberString(s string) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "đ", "")
	s = strings.ReplaceAll(s, "vnd", "")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0
	}
	if strings.Count(s, ".") > 1 || (strings.Count(s, ".") == 1 && len(s)-strings.LastIndex(s, ".")-1 == 3) {
		s = strings.ReplaceAll(s, ".", "")
	}
	if strings.Count(s, ",") > 1 || (strings.Count(s, ",") == 1 && len(s)-strings.LastIndex(s, ",")-1 == 3) {
		s = strings.ReplaceAll(s, ",", "")
	} else {
		s = strings.ReplaceAll(s, ",", ".")
	}
	n, _ := strconv.ParseFloat(s, 64)
	return n
}

func specialOfferForWeekday(raw json.RawMessage, weekdayIdx int) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		if weekdayIdx >= 0 && weekdayIdx < len(arr) {
			return formatSpecialOffer(arr[weekdayIdx])
		}
		return ""
	}

	var byDay map[string]json.RawMessage
	if json.Unmarshal(raw, &byDay) == nil {
		for _, key := range []string{"1", "2", "monday", "mon", "thu_2", "thứ 2", "Thứ 2"} {
			if v, ok := byDay[key]; ok {
				return formatSpecialOffer(v)
			}
		}
		keys := make([]string, 0, len(byDay))
		for key := range byDay {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var parts []string
		for _, key := range keys {
			var offerByDay map[string]json.RawMessage
			if json.Unmarshal(byDay[key], &offerByDay) != nil {
				continue
			}
			for _, dayKey := range weekdayKeys(weekdayIdx) {
				if v, ok := offerByDay[dayKey]; ok {
					if text := formatSpecialOffer(v); text != "" {
						parts = append(parts, formatOfferTimeKey(key)+": "+text)
					}
					break
				}
			}
		}
		return strings.Join(parts, " · ")
	}

	return ""
}

func weekdayKeys(weekdayIdx int) []string {
	switch weekdayIdx {
	case 1:
		return []string{"1", "2", "monday", "mon", "thu_2", "thứ 2", "Thứ 2"}
	default:
		return []string{strconv.Itoa(weekdayIdx)}
	}
}

func formatOfferTimeKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) == 4 {
		start, errStart := strconv.Atoi(key[:2])
		end, errEnd := strconv.Atoi(key[2:])
		if errStart == nil && errEnd == nil {
			return fmt.Sprintf("%d-%dh", start, end)
		}
	}
	return key
}

func formatSpecialOffer(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var n float64
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		return formatVND(int64(math.Round(n))) + " đ"
	}

	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}

	var obj map[string]interface{}
	if json.Unmarshal(raw, &obj) == nil {
		label := firstString(obj, "name", "label", "title", "time", "times", "duration")
		price := firstNumber(obj, "price", "amount", "value", "total_price", "rate")
		switch {
		case label != "" && price > 0:
			return fmt.Sprintf("%s - %s đ", label, formatVND(int64(math.Round(price))))
		case price > 0:
			return formatVND(int64(math.Round(price))) + " đ"
		case label != "":
			return label
		}
	}

	return ""
}

// firstNonEmptyStrings trả về slice đầu tiên không rỗng, thử lần lượt mỗi raw.
func firstNonEmptyStrings(rawList ...json.RawMessage) []string {
	for _, raw := range rawList {
		if items := jsonStringSlice(raw); len(items) > 0 {
			return items
		}
	}
	return nil
}

// jsonStringSlice parse mảng string từ JSON. Hỗ trợ:
//   - ["a", "b", "c"]
//   - [{"name":"a"}, {"label":"b"}, {"title":"c"}, {"text":"d"}]
//   - "a- b- c" hoặc "a\nb\nc"
func jsonStringSlice(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		out := make([]string, 0, len(arr))
		for _, s := range arr {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	var objs []map[string]interface{}
	if json.Unmarshal(raw, &objs) == nil {
		out := make([]string, 0, len(objs))
		for _, o := range objs {
			if s := firstString(o, "name", "label", "title", "text"); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		fields := strings.FieldsFunc(s, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ';' || r == ','
		})
		out := make([]string, 0, len(fields))
		for _, f := range fields {
			if f = strings.TrimSpace(f); f != "" {
				out = append(out, f)
			}
		}
		return out
	}
	return nil
}

func firstString(obj map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := obj[key].(string); ok {
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func firstNumber(obj map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		switch v := obj[key].(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0
}

// formatVND format số tiền VND có dấu chấm phân tách
func formatVND(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	return string(out)
}

// ─── Handlers ─────────────────────────────────────────────────────────────

// POST /api/parse-listings — trả về danh sách listings (không download ảnh)
func parseListingsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var body struct {
			URL  string          `json:"url"`
			JSON json.RawMessage `json:"json"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			log.Printf("[parse-listings] invalid request body: %v", err)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "invalid body",
				"details": err.Error(),
			})
			return
		}

		source := "inline JSON"
		if strings.TrimSpace(body.URL) != "" {
			source = body.URL
		}
		log.Printf("[parse-listings] request source=%s inline_json_bytes=%d", source, len(body.JSON))

		rawJSON, err := fetchRawJSON(body.URL, body.JSON)
		if err != nil {
			log.Printf("[parse-listings] fetch failed source=%s error=%v", source, err)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "Không tải được API",
				"details": err.Error(),
			})
			return
		}
		log.Printf("[parse-listings] fetched source=%s bytes=%d preview=%q", source, len(rawJSON), rawPreview(rawJSON, 500))

		listings := parseListings(rawJSON)
		if len(listings) == 0 {
			details := "Parser không tìm thấy mảng listing có ảnh. Kiểm tra response có chứa listings/items/results/data và ảnh trong pictures hoặc photo_urls không."
			log.Printf("[parse-listings] parse empty source=%s bytes=%d preview=%q", source, len(rawJSON), rawPreview(rawJSON, 1000))
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "không tìm thấy listing nào có ảnh",
				"details": details,
			})
			return
		}

		log.Printf("[parse-listings] parsed source=%s listings=%d", source, len(listings))
		json.NewEncoder(w).Encode(map[string]interface{}{
			"listings": listings,
			"count":    len(listings),
		})
	}
}

// dayladauListingsHandler builds the dayladau v1/listings URL from date/guest
// params and returns parsed listings. The app is dayladau-only, so this saves
// the user from pasting the full API URL every time.
func dayladauListingsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var body struct {
			Checkin  string `json:"checkin"`
			Checkout string `json:"checkout"`
			Guests   int    `json:"guests"`
			Limit    int    `json:"limit"`
			Offset   int    `json:"offset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			log.Printf("[dayladau] invalid request body: %v", err)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid body", "details": err.Error()})
			return
		}

		apiURL := buildDayladauURL(body.Checkin, body.Checkout, body.Guests, body.Limit, body.Offset)
		log.Printf("[dayladau] fetch url=%s", apiURL)

		rawJSON, err := fetchRawJSON(apiURL, nil)
		if err != nil {
			log.Printf("[dayladau] fetch failed url=%s error=%v", apiURL, err)
			json.NewEncoder(w).Encode(map[string]string{"error": "Không tải được Dayladau", "details": err.Error()})
			return
		}

		listings := parseListings(rawJSON)
		if len(listings) == 0 {
			log.Printf("[dayladau] parse empty bytes=%d preview=%q", len(rawJSON), rawPreview(rawJSON, 600))
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "không tìm thấy listing nào",
				"details": "Dayladau không trả listing có ảnh cho khoảng thời gian/số khách này.",
			})
			return
		}

		log.Printf("[dayladau] parsed listings=%d", len(listings))
		json.NewEncoder(w).Encode(map[string]interface{}{"listings": listings, "count": len(listings)})
	}
}

// buildDayladauURL constructs the dayladau v1/listings query. It drops the
// shorten flag so each listing returns its full photo gallery (needed for a
// video), and uses a fresh _updated timestamp as a cache-buster.
func buildDayladauURL(checkin, checkout string, guests, limit, offset int) string {
	if guests <= 0 {
		guests = 2
	}
	if limit <= 0 {
		limit = 20
	}
	if offset <= 0 {
		offset = 1
	}
	now := time.Now()
	if strings.TrimSpace(checkin) == "" {
		checkin = now.Format("2006-01-02")
	}
	if strings.TrimSpace(checkout) == "" {
		checkout = now.AddDate(0, 0, 1).Format("2006-01-02")
	}

	q := url.Values{}
	q.Set("_updated", strconv.FormatInt(now.UnixMilli(), 10))
	q.Set("number_of_guests", strconv.Itoa(guests))
	q.Set("is_suggest_special_offer_time", "false")
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("start_time_v2", dayladauTime(checkin, "1400"))
	q.Set("end_time_v2", dayladauTime(checkout, "1100"))
	q.Set("is_suggest_promotion", "true")
	return "https://api.dayladau.com/v1/listings?" + q.Encode()
}

// dayladauTime turns a "2026-06-23" date into the API's "2026-06-23T1400"
// form; values that already carry a T<time> suffix pass through unchanged.
func dayladauTime(date, hour string) string {
	date = strings.TrimSpace(date)
	if date == "" || strings.Contains(date, "T") {
		return date
	}
	return date + "T" + hour
}

func rawPreview(raw []byte, limit int) string {
	s := strings.TrimSpace(string(raw))
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "...(truncated)"
}

// Download tuning for listing photos. Concurrency is capped so a big batch
// (many listings × many photos) doesn't exhaust connections/FDs — which made
// later listings in a batch fail. Each photo retries transient failures.
const (
	maxConcurrentDownloads = 6
	maxDownloadAttempts    = 3
	perAttemptTimeout      = 15 * time.Second
)

// imageDownloadClient downloads listing photos. A var (not const) so tests can
// shorten the timeout. The per-attempt context is the precise cap; Timeout is a
// backstop. Connection-pool limits avoid socket exhaustion across a batch.
var imageDownloadClient = &http.Client{
	Timeout: 25 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: maxConcurrentDownloads,
		MaxConnsPerHost:     maxConcurrentDownloads * 2,
		IdleConnTimeout:     30 * time.Second,
	},
}

// fetchPhotoWithRetry downloads imgURL into uploadDir/%04d<ext>, retrying
// transient failures (network errors, timeouts, 429/5xx) with backoff. It
// writes to a temp file and atomically renames into place so collectImages
// never observes a half-written or partial image. Honors ctx cancellation so a
// client disconnect aborts in-flight downloads instead of leaking orphan
// writers into a dir the next listing will reuse.
func fetchPhotoWithRetry(ctx context.Context, imgURL, uploadDir string, idx int) error {
	var lastErr error
	for attempt := 0; attempt < maxDownloadAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 600 * time.Millisecond):
			}
		}
		retry, err := fetchPhotoOnce(ctx, imgURL, uploadDir, idx)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry || ctx.Err() != nil {
			break
		}
	}
	return lastErr
}

// fetchPhotoOnce performs one download attempt. retry is true when the failure
// is transient and worth another attempt.
func fetchPhotoOnce(ctx context.Context, imgURL, uploadDir string, idx int) (retry bool, err error) {
	reqCtx, cancel := context.WithTimeout(ctx, perAttemptTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, imgURL, nil)
	if err != nil {
		return false, err // malformed URL — permanent
	}
	resp, err := imageDownloadClient.Do(req)
	if err != nil {
		// A timeout means the photo is slow; retrying within the same tight
		// per-request budget just stalls the batch again (and compounds the
		// wall-clock — the very hang this guards against). Retry only quick
		// connection blips, and only while the parent isn't cancelled.
		return ctx.Err() == nil && !isTimeoutErr(err), err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Don't write a 403/404 HTML error page out as a .jpg — ffmpeg would
		// choke on it. 408/429/5xx are transient; other 4xx are permanent.
		transient := resp.StatusCode == http.StatusRequestTimeout ||
			resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode >= 500
		return transient, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	ext := pickPhotoExt(imgURL, resp.Header.Get("Content-Type"))
	tmp, err := os.CreateTemp(uploadDir, ".dl-*.tmp")
	if err != nil {
		return false, err
	}
	tmpName := tmp.Name()
	_, copyErr := io.Copy(tmp, resp.Body)
	tmp.Close()
	if copyErr != nil {
		os.Remove(tmpName) // partial body — retry into a fresh temp file
		return ctx.Err() == nil && !isTimeoutErr(copyErr), copyErr
	}
	if err := os.Rename(tmpName, filepath.Join(uploadDir, fmt.Sprintf("%04d%s", idx, ext))); err != nil {
		os.Remove(tmpName)
		return false, err
	}
	return false, nil
}

// isTimeoutErr reports whether err is a request/deadline timeout (client
// Timeout, per-attempt context deadline, or a net timeout) — as opposed to a
// quick connection error. Slow timeouts aren't worth retrying within one
// request; quick connection errors are.
func isTimeoutErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var nerr net.Error
	return errors.As(err, &nerr) && nerr.Timeout()
}

// pickPhotoExt resolves a usable image extension from the URL or content-type.
func pickPhotoExt(imgURL, contentType string) string {
	ext := filepath.Ext(strings.Split(imgURL, "?")[0])
	if ext != "" && len(ext) <= 5 {
		return ext
	}
	switch {
	case strings.Contains(contentType, "jpeg"):
		return ".jpg"
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	default:
		return ".jpg"
	}
}

// POST /api/select-listing — download ảnh của 1 listing đã chọn (song song có
// giới hạn, retry lỗi tạm thời, fail-closed nếu không tải được ảnh nào).
func selectListingHandler(uploadDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var body struct {
			PhotoURLs   []string `json:"photo_urls"`
			ListingID   string   `json:"listing_id"`
			ListingName string   `json:"listing_name"`
			Address     string   `json:"address"`
			PhotoLimit  int      `json:"photo_limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.PhotoURLs) == 0 {
			json.NewEncoder(w).Encode(map[string]string{"error": "thiếu photo_urls"})
			return
		}
		if body.PhotoLimit > 0 && body.PhotoLimit < len(body.PhotoURLs) {
			body.PhotoURLs = body.PhotoURLs[:body.PhotoLimit]
		}

		// Lưu context listing hiện tại
		setCurrentListing(body.ListingName, body.ListingID, body.Address, len(body.PhotoURLs))

		// Đừng xoá thư mục ảnh chung khi một render đang đọc nó. Frontend đáng lẽ
		// render tuần tự, nhưng nếu hàng chờ lỡ nhảy sớm thì chờ render hiện tại
		// xong (tối đa ~30s) để không rút ảnh khỏi tay ffmpeg đang chạy.
		for i := 0; i < 300; i++ {
			state.mu.Lock()
			running := state.running
			state.mu.Unlock()
			if !running {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		os.RemoveAll(uploadDir)
		os.MkdirAll(uploadDir, 0755)

		type dlResult struct {
			idx int
			err error
		}
		ch := make(chan dlResult, len(body.PhotoURLs))
		sem := make(chan struct{}, maxConcurrentDownloads)

		for i, u := range body.PhotoURLs {
			go func(idx int, imgURL string) {
				sem <- struct{}{}
				defer func() { <-sem }()
				ch <- dlResult{idx, fetchPhotoWithRetry(r.Context(), imgURL, uploadDir, idx)}
			}(i, u)
		}

		success, errs := 0, []string{}
		for range body.PhotoURLs {
			res := <-ch
			if res.err != nil {
				errs = append(errs, fmt.Sprintf("%d: %v", res.idx+1, res.err))
			} else {
				success++
			}
		}

		total := len(body.PhotoURLs)
		// Fail-closed: with zero usable images the render would later die with
		// "không có ảnh". Surface an explicit error so the frontend marks the
		// listing ✗ cleanly and never starts a doomed render.
		if success == 0 {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"count":  0,
				"total":  total,
				"error":  fmt.Sprintf("không tải được ảnh nào (0/%d)", total),
				"errors": errs,
			})
			return
		}

		resp := map[string]interface{}{
			"count":  success,
			"total":  total,
			"errors": errs,
		}
		if success < total {
			resp["warning"] = fmt.Sprintf("chỉ tải được %d/%d ảnh", success, total)
		}
		json.NewEncoder(w).Encode(resp)
	}
}

// fetchListingHandler — giữ lại cho compat (POST /api/fetch-listing)
func fetchListingHandler(uploadDir string) http.HandlerFunc {
	return selectListingHandler(uploadDir)
}

func fetchRawJSON(apiURL string, inline json.RawMessage) ([]byte, error) {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL != "" {
		parsedURL, err := url.Parse(apiURL)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			return nil, fmt.Errorf("URL API không hợp lệ: %q. Hãy nhập URL đầy đủ dạng https://... hoặc dán JSON response vào ô JSON", apiURL)
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return nil, fmt.Errorf("URL API phải bắt đầu bằng http:// hoặc https://, hiện tại là %q", parsedURL.Scheme)
		}
		client := &http.Client{Timeout: 45 * time.Second}
		req, err := http.NewRequest(http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("URL API không hợp lệ: %v", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "img2video/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("không fetch được URL: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("API trả HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	if len(inline) == 0 || string(inline) == "null" {
		return nil, fmt.Errorf("thiếu URL API hoặc JSON")
	}
	return inline, nil
}

// extractPictureURLs — giữ lại cho compat
func extractPictureURLs(data []byte) []string {
	listings := parseListings(data)
	var urls []string
	for _, l := range listings {
		urls = append(urls, l.PhotoURLs...)
	}
	return urls
}
