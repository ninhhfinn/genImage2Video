package main

// Đồng bộ phòng từ Dayladau + sinh ca dọn.
//
// PHẠM VI THẬT / CHƯA THẬT — đọc kỹ trước khi tin số liệu:
//
//   • Danh sách phòng: THẬT. Lấy từ https://api.dayladau.com/v1/listings, cùng
//     endpoint mà công cụ video trong repo này đang dùng (backend/listings.go).
//     Tên phòng, địa chỉ, quận, số phòng ngủ, giờ nhận/trả phòng, chủ nhà đều là
//     dữ liệu sản xuất.
//
//   • Lịch check-out từng ngày: CHƯA THẬT. Endpoint trên là API TÌM PHÒNG (phòng
//     nào còn trống), không phải lịch đặt. Muốn biết "hôm nay phòng nào khách trả"
//     phải gọi API reservations của host — endpoint đó cần xác thực và hiện chưa
//     có token. Chỗ cắm vào đã chừa sẵn ở hkFetchCheckouts().
//
// Vì vậy ca dọn hiện được tạo bằng hai đường: quản lý tự thêm, hoặc sinh theo
// lịch mô phỏng cho môi trường demo. Cả hai đều đi qua đúng một hàm tạo ca, nên
// lúc cắm API thật vào thì phần còn lại không phải sửa.

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ─── Đồng bộ phòng ────────────────────────────────────────────────────────

type hkListingHost struct {
	ID       string `json:"id"`
	Fullname string `json:"fullname"`
	Phone    string `json:"phone"`
}

// hkCheckinGuide — hướng dẫn nhận phòng của Dayladau là nội dung có cấu trúc
// (khối chữ xen khối ảnh), không phải một chuỗi. Cô dọn dẹp cần phần chữ (mã
// khoá, chỉ đường); ảnh minh hoạ bỏ qua vì màn của cô đã chật.
type hkCheckinGuide struct {
	Blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"blocks"`
}

func (g hkCheckinGuide) plainText() string {
	var parts []string
	for _, b := range g.Blocks {
		if b.Type != "text" {
			continue
		}
		if t := strings.TrimSpace(b.Text); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " · ")
}

type hkRawListing struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Nickname    string `json:"nickname"`
	FullAddress string `json:"full_address"`
	District    string `json:"district"`
	City        string `json:"city"`
	Bedrooms    int    `json:"bedrooms"`
	CheckinHour  int            `json:"checkin_hour"`
	CheckoutHour int            `json:"checkout_hour"`
	CheckinGuide hkCheckinGuide `json:"checkin_guide"`
	CleanTime    int            `json:"clean_time"` // đệm dọn dẹp giữa hai lượt khách, giờ
	Status       string         `json:"status"`
	Host         hkListingHost  `json:"host"`
}

// Giải mã từng listing riêng, không giải mã cả mảng một lượt.
//
// Dữ liệu sản xuất có bản ghi lệch kiểu (một trường đáng lẽ là chuỗi lại là
// object, giờ là null…). Giải mã cả mảng thì MỘT căn lạ làm hỏng toàn bộ lượt
// đồng bộ; giải mã từng cái thì bỏ qua căn đó và 59 căn còn lại vẫn về.
type hkListingsResponse struct {
	Listings []json.RawMessage `json:"listings"`
	Total    int               `json:"total"`
}

// hkSyncRooms kéo listing từ Dayladau và upsert vào bảng phòng.
//
// Trả về số phòng mới và số phòng cập nhật. Trường do quản lý chỉnh tay (đơn giá,
// mẫu checklist, hướng dẫn vào nhà) KHÔNG bị ghi đè — xem HKStore.UpsertRoom.
func (a *HKApp) hkSyncRooms(limit int) (added, updated int, err error) {
	if limit <= 0 {
		limit = 60
	}
	now := time.Now()
	// Dùng khoảng ngày mai→ngày kia: API trả phòng còn trống trong khoảng đó.
	// Lấy khoảng gần để danh sách bám sát những căn đang thật sự bán.
	checkin := now.AddDate(0, 0, 1).Format("2006-01-02")
	checkout := now.AddDate(0, 0, 2).Format("2006-01-02")
	// Ba tham số địa điểm để rỗng: module dọn dẹp cần TẤT CẢ phòng đang bán, không
	// lọc theo tỉnh/phường như màn dựng video.
	apiURL := buildDayladauURL(checkin, checkout, 2, limit, 1, "", "", "")

	raw, err := fetchRawJSON(apiURL, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("không tải được danh sách phòng Dayladau: %w", err)
	}
	var resp hkListingsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, 0, fmt.Errorf("không đọc được phản hồi Dayladau: %w", err)
	}
	if len(resp.Listings) == 0 {
		return 0, 0, fmt.Errorf("Dayladau không trả phòng nào cho khoảng %s → %s", checkin, checkout)
	}

	existing, err := a.store.ListRooms(false)
	if err != nil {
		return 0, 0, err
	}
	known := map[string]bool{}
	for _, r := range existing {
		known[r.ID] = true
	}

	templates, err := a.store.ListTemplates()
	if err != nil {
		return 0, 0, err
	}

	ts := hkNowMs()
	skipped := 0
	for _, rawItem := range resp.Listings {
		var l hkRawListing
		if err := json.Unmarshal(rawItem, &l); err != nil {
			skipped++
			log.Printf("[hk] bỏ qua một listing không đọc được: %v", err)
			continue
		}
		if strings.TrimSpace(l.ID) == "" {
			skipped++
			continue
		}
		roomType := hkRoomTypeFromBedrooms(l.Bedrooms)
		room := HKRoom{
			ID:         l.ID,
			ListingID:  l.ID,
			Code:       hkRoomCode(l),
			Name:       hkRoomName(l),
			Address:    strings.TrimSpace(l.FullAddress),
			Zone:       hkZoneOf(l),
			RoomType:   roomType,
			HostID:     l.Host.ID,
			HostName:   strings.TrimSpace(l.Host.Fullname),
			TemplateID: hkTemplateForRoomType(templates, roomType),
			DoorNote:   hkTrimGuide(l.CheckinGuide.plainText()),
			CleanTime:  hkHourOr(l.CleanTime, 1),
			CheckinHr:  hkHourOr(l.CheckinHour, 14),
			CheckoutHr: hkHourOr(l.CheckoutHour, 11),
			Active:     true,
			SyncedAt:   ts,
		}
		if err := a.store.UpsertRoom(room); err != nil {
			log.Printf("[hk] upsert phòng %s lỗi: %v", room.ID, err)
			continue
		}
		if known[room.ID] {
			updated++
		} else {
			added++
		}
	}
	// Bỏ qua bao nhiêu căn phải nói ra: im lặng thì "đồng bộ 40 phòng" trong khi
	// Dayladau có 60 trông y hệt như đã lấy đủ.
	if skipped > 0 {
		log.Printf("[hk] đồng bộ xong: thêm %d, cập nhật %d, bỏ qua %d listing lỗi", added, updated, skipped)
	}
	return added, updated, nil
}

func hkRoomName(l hkRawListing) string {
	if n := strings.TrimSpace(l.Nickname); n != "" {
		return n
	}
	name := strings.TrimSpace(l.Name)
	// Tên listing trên Dayladau là tên bán hàng, dài và nhồi từ khoá
	// ("… Máy chiếu Netflix/Bếp riêng/wc khép kín/Để xe trong tòa nhà"). Cô dọn
	// dẹp cần nhận ra căn nhà, không cần đọc quảng cáo — cắt ở dấu phân cách đầu.
	for _, sep := range []string{".", "|", "–", "-"} {
		if i := strings.Index(name, sep); i > 8 {
			name = strings.TrimSpace(name[:i])
			break
		}
	}
	if len([]rune(name)) > 60 {
		name = string([]rune(name)[:60]) + "…"
	}
	if name == "" {
		return l.ID
	}
	return name
}

func hkRoomCode(l hkRawListing) string {
	id := strings.TrimSpace(l.ID)
	if len(id) > 6 {
		id = id[len(id)-6:]
	}
	zone := hkZoneOf(l)
	if zone == "" {
		return strings.ToUpper(id)
	}
	return hkZoneAbbr(zone) + "-" + strings.ToUpper(id)
}

// hkZoneAbbr rút quận thành 2-3 chữ cái đầu của mỗi từ: "Cầu Giấy" → "CG".
func hkZoneAbbr(zone string) string {
	var b strings.Builder
	for _, w := range strings.Fields(zone) {
		r := []rune(w)
		if len(r) > 0 {
			b.WriteString(strings.ToUpper(string(r[0])))
		}
	}
	s := b.String()
	if len([]rune(s)) > 3 {
		s = string([]rune(s)[:3])
	}
	return s
}

func hkZoneOf(l hkRawListing) string {
	if d := strings.TrimSpace(l.District); d != "" {
		return d
	}
	return strings.TrimSpace(l.City)
}

func hkTrimGuide(s string) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > 240 {
		s = string([]rune(s)[:240]) + "…"
	}
	return s
}

func hkHourOr(h, fallback int) int {
	if h <= 0 || h > 23 {
		return fallback
	}
	return h
}

func hkTemplateForRoomType(templates []HKTemplate, roomType string) string {
	for _, t := range templates {
		for _, rt := range t.RoomTypes {
			if rt == roomType {
				return t.ID
			}
		}
	}
	if len(templates) > 0 {
		return templates[0].ID
	}
	return ""
}

// ─── Sinh ca dọn từ lịch đặt phòng ────────────────────────────────────────

// hkSyncSessions đọc iCal của mọi phòng đang vận hành và tạo ca dọn cho các lượt
// khách trả phòng trong khoảng [hôm nay − 1, hôm nay + ahead] ngày.
//
// Lùi 1 ngày để bắt lượt trả phòng hôm qua mà chưa ai dọn — nếu chỉ nhìn từ hôm
// nay thì ca bị bỏ sót sẽ biến mất khỏi màn hình thay vì hiện ra để xử lý.
//
// Chạy lại bao nhiêu lần cũng không đẻ ca trùng: khoá là mã lượt đặt trong iCal.
func (a *HKApp) hkSyncSessions(ahead int) (created int, skipped int, assigned int, err error) {
	if ahead <= 0 {
		ahead = 14
	}
	rooms, err := a.store.ListRooms(true)
	if err != nil {
		return 0, 0, 0, err
	}
	if len(rooms) == 0 {
		return 0, 0, 0, fmt.Errorf("chưa có phòng nào — bấm Đồng bộ phòng trước")
	}

	loc := time.Now().Location()
	today := time.Now().In(loc)
	from := today.AddDate(0, 0, -1).Format("2006-01-02")
	to := today.AddDate(0, 0, ahead).Format("2006-01-02")
	client := &http.Client{Timeout: 25 * time.Second}

	var failed int
	for _, room := range rooms {
		events, e := hkFetchICal(client, room.ListingID, loc)
		if e != nil {
			// Một feed hỏng không được làm hỏng cả lượt đồng bộ.
			failed++
			log.Printf("[hk] iCal %s lỗi: %v", room.ListingID, e)
			continue
		}
		for _, turn := range hkTurnsFromEvents(room, events, room.CleanTime, loc) {
			if turn.Day < from || turn.Day > to {
				continue
			}
			ok, e := a.hkCreateSessionFromTurn(room, turn)
			if e != nil {
				log.Printf("[hk] tạo ca %s lỗi: %v", turn.UID, e)
				continue
			}
			if ok {
				created++
			} else {
				skipped++
			}
		}
	}
	if failed > 0 {
		log.Printf("[hk] đồng bộ lịch: tạo %d ca, bỏ qua %d ca đã có, %d phòng lỗi feed", created, skipped, failed)
	}

	// Gợi ý lại người phụ trách cho ca CŨ còn trống.
	//
	// Gợi ý chỉ chạy lúc tạo ca là không đủ: ca sinh ra trước khi có cô nào nhận
	// khu đó thì mãi mãi ở trạng thái "chưa xếp", và bấm Đồng bộ lịch lần nữa
	// cũng không sửa được vì ca đã tồn tại. Quản lý ngồi nhìn một danh sách trống
	// người mà không hiểu vì sao — đúng tình huống đã gặp khi test.
	//
	// Chỉ đụng ca chưa ai bắt đầu: ca đang dở mà bị đổi người là cướp việc giữa chừng.
	assigned, aerr := a.hkSuggestStaffForOpenSessions(from, to)
	if aerr != nil {
		// Gán người là việc phụ; hỏng ở đây không được làm hỏng cả lượt đồng bộ.
		log.Printf("[hk] gợi ý người phụ trách lỗi: %v", aerr)
		assigned = 0
	} else if assigned > 0 {
		log.Printf("[hk] đã gợi ý người phụ trách cho %d ca còn trống", assigned)
	}
	return created, skipped, assigned, nil
}

// hkSuggestStaffForOpenSessions gán người theo khu vực cho các ca còn trống.
// Trả về số ca đã gán được.
func (a *HKApp) hkSuggestStaffForOpenSessions(fromDay, toDay string) (int, error) {
	open, err := a.store.ListUnassignedSessions(fromDay, toDay)
	if err != nil {
		return 0, err
	}
	if len(open) == 0 {
		return 0, nil
	}
	rooms, err := a.store.ListRooms(false)
	if err != nil {
		return 0, err
	}
	zoneOf := map[string]string{}
	for _, r := range rooms {
		zoneOf[r.ID] = r.Zone
	}

	n := 0
	for _, sess := range open {
		staffID := a.hkSuggestStaff(zoneOf[sess.RoomID])
		if staffID == "" {
			continue
		}
		if err := a.store.AssignSessionStaff(sess.ID, staffID); err != nil {
			log.Printf("[hk] gán ca %s lỗi: %v", sess.ID, err)
			continue
		}
		n++
	}
	return n, nil
}

// hkCreateSessionFromTurn tạo một ca dọn từ một lượt khách rời phòng.
// Trả false khi lượt đó đã có ca — không phải lỗi, chỉ là đồng bộ chạy lại.
func (a *HKApp) hkCreateSessionFromTurn(room HKRoom, turn hkTurn) (bool, error) {
	var snapshot *HKTemplate
	if room.TemplateID != "" {
		if t, err := a.store.TemplateByID(room.TemplateID); err == nil {
			snapshot = &t
		}
	}
	nextCheckin := int64(0)
	if turn.HasNext {
		nextCheckin = turn.DeadlineAt.UnixMilli()
	}
	sess := HKSession{
		ID:               "hks_" + hkShortHash(turn.UID),
		Day:              turn.Day,
		RoomID:           room.ID,
		ListingID:        room.ListingID,
		TemplateID:       room.TemplateID,
		BookingUID:       turn.UID,
		Status:           HKSessionTodo,
		CheckoutAt:       turn.CheckoutAt.UnixMilli(),
		NextCheckinAt:    nextCheckin,
		DeadlineAt:       turn.DeadlineAt.UnixMilli(),
		ItemsState:       map[string]HKItemState{},
		TemplateSnapshot: snapshot,
		StaffID:          a.hkSuggestStaff(room.Zone),
	}
	return a.store.InsertSessionIfAbsent(sess)
}

// hkShortHash rút mã lượt đặt dài của iCal thành id ngắn, ổn định.
func hkShortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// hkSuggestStaff chọn cô đang làm và nhận khu vực này. Trả rỗng khi không ai
// khớp — để trống cho quản lý xếp tay, tốt hơn là gán bừa cho người ở quận khác.
func (a *HKApp) hkSuggestStaff(zone string) string {
	users, err := a.store.ListUsers(HKRoleCleaner)
	if err != nil {
		return ""
	}
	for _, u := range users {
		if u.Status != HKStaffActive {
			continue
		}
		for _, z := range u.Zones {
			if strings.EqualFold(strings.TrimSpace(z), strings.TrimSpace(zone)) {
				return u.ID
			}
		}
	}
	return ""
}
