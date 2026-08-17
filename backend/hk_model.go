package main

// Module Dọn dẹp (housekeeping) — kiểu dữ liệu và luật nghiệp vụ.
//
// File này cố tình KHÔNG đụng tới database hay HTTP: đây là chỗ quyết định "đủ
// ảnh chưa" và "trả bao nhiêu tiền", tức là những phép tính ra tiền cho người
// thật, nên nó phải test được mà không cần dựng server hay bảng.

import "strings"

// ─── Trạng thái ───────────────────────────────────────────────────────────

const (
	HKSessionTodo       = "todo"
	HKSessionInProgress = "in_progress"
	HKSessionSubmitted  = "submitted" // đủ ảnh, công đã ghi, chờ quản lý đối soát
	HKSessionApproved   = "approved"
	HKSessionRejected   = "rejected"
)

const (
	HKStaffPending   = "pending"
	HKStaffActive    = "active"
	HKStaffSuspended = "suspended"
	HKStaffRejected  = "rejected"
)

const (
	HKRoleAdmin   = "admin"
	HKRoleCleaner = "cleaner"
)

// defaultMinPhotos để 1 chứ không phải 2: mỗi ảnh thêm là thêm ~20 giây thao tác
// trên điện thoại yếu sóng, nhân 14 mục × 8 phòng/ngày thì thành nửa tiếng công
// không ai trả. Mục nào thật sự cần 2 góc thì đặt riêng MinPhotos ở mẫu.
const defaultMinPhotos = 1

// ─── Kiểu dữ liệu ─────────────────────────────────────────────────────────

type HKUser struct {
	ID        string   `json:"id"`
	Role      string   `json:"role"` // admin | cleaner
	Name      string   `json:"name"`
	Phone     string   `json:"phone"`
	Status    string   `json:"status"`
	Zones     []string `json:"zones"`
	Note      string   `json:"note"`
	Bank      string   `json:"bank"`
	CreatedAt int64    `json:"created_at"`
	// PasswordHash không bao giờ ra khỏi backend — không có tag json để lỡ tay
	// encode cả struct cũng không lộ.
	PasswordHash string `json:"-"`
}

type HKRoom struct {
	ID         string `json:"id"`
	ListingID  string `json:"listing_id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	Zone       string `json:"zone"`
	RoomType   string `json:"room_type"`
	HostID     string `json:"host_id"`
	HostName   string `json:"host_name"`
	TemplateID string `json:"template_id"`
	DoorNote   string `json:"door_note"`
	// Đệm dọn dẹp tối thiểu giữa hai lượt khách, lấy từ listing (`clean_time`).
	CleanTime int `json:"clean_time"`
	// Cơ sở: Dayladau chỉ trả về mã số, không trả tên. Nhãn được suy từ địa chỉ
	// chung của các phòng cùng cơ sở — "Cơ sở #309" thì người vận hành không biết
	// là chỗ nào, còn "Ngõ 387 Vũ Tông Phan" thì biết ngay.
	FacilityID    int    `json:"facility_id"`
	FacilityLabel string `json:"facility_label"`
	CheckinHr  int    `json:"checkin_hour"`
	CheckoutHr int    `json:"checkout_hour"`
	Active     bool   `json:"active"`
	SyncedAt   int64  `json:"synced_at"`
}

type HKItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// RequirePhoto luôn true — giữ trường để đọc được dữ liệu bản cũ, xem
	// hkNormalizeTemplate.
	RequirePhoto bool `json:"require_photo"`
	MinPhotos    int  `json:"min_photos,omitempty"`
	// Hint: mô tả yêu cầu — dọn xong phải trông thế nào, chụp góc nào.
	Hint string `json:"hint,omitempty"`
	// SamplePhoto: ảnh mẫu quản lý chụp sẵn, hiện ngay trên màn chụp của cô.
	// Một tấm ảnh nói rõ "đạt yêu cầu là thế này" hơn ba dòng chữ mô tả.
	SamplePhoto string `json:"sample_photo,omitempty"`
}

type HKGroup struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Items []HKItem `json:"items"`
}

type HKTemplate struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	RoomTypes []string  `json:"room_types"`
	Groups    []HKGroup `json:"groups"`
	UpdatedAt int64     `json:"updated_at"`
}

type HKPhoto struct {
	URL        string `json:"url"`
	UploadedAt int64  `json:"uploaded_at"`
}

type HKItemState struct {
	Photos    []HKPhoto `json:"photos,omitempty"`
	Checked   bool      `json:"checked,omitempty"`
	DoneAt    int64     `json:"done_at,omitempty"`
	CheckedAt int64     `json:"checked_at,omitempty"`
}

type HKSession struct {
	ID         string `json:"id"`
	Day        string `json:"day"` // YYYY-MM-DD
	RoomID     string `json:"room_id"`
	ListingID  string `json:"listing_id"`
	TemplateID string `json:"template_id"`
	StaffID    string `json:"staff_id"`
	Status     string `json:"status"`

	CheckoutAt    int64 `json:"checkout_at"`
	NextCheckinAt int64 `json:"next_checkin_at"`
	DeadlineAt    int64 `json:"deadline_at"`

	GuestNote string `json:"guest_note"`
	// Cô báo hỏng hóc / thiếu đồ. Nằm NGOÀI checklist: nó không phải một việc phải
	// làm, và bắt cô đi qua nó mỗi ca chỉ để bấm "không có gì" là thêm một chạm vô
	// ích cho phần lớn số ca.
	CleanerNote string `json:"cleaner_note"`

	// Mã lượt đặt lấy từ iCal — khoá chống trùng khi đồng bộ chạy lại.
	// Phải là mã lượt chứ không phải (phòng, ngày): 59/60 phòng cho thuê theo
	// giờ nên một phòng có thể có nhiều lượt khách trong cùng một ngày, mỗi lượt
	// là một ca dọn kỹ riêng.
	BookingUID string `json:"booking_uid"`

	ItemsState map[string]HKItemState `json:"items_state"`

	StartedAt   int64  `json:"started_at"`
	SubmittedAt int64  `json:"submitted_at"`
	ReviewedAt  int64  `json:"reviewed_at"`
	ReviewedBy  string `json:"reviewed_by"`
	ReviewNote  string `json:"review_note"`

	// Ảnh chụp mẫu checklist tại thời điểm tạo ca. Sửa mẫu KHÔNG được đổi điều
	// kiện của ca đang dở — nếu không, cô đang dọn dở bỗng thiếu ảnh cho một mục
	// mà lúc bắt đầu chưa hề tồn tại.
	TemplateSnapshot *HKTemplate `json:"template_snapshot,omitempty"`
}

// ─── Loại phòng ───────────────────────────────────────────────────────────
//
// Chỉ còn để gán mẫu checklist — 2 phòng ngủ nhiều việc hơn studio. Không còn
// đơn giá vì phần mềm này không tính lương.

type HKRoomType struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

var hkRoomTypes = []HKRoomType{
	{Key: "studio", Label: "Studio"},
	{Key: "one_bedroom", Label: "1 phòng ngủ"},
	{Key: "two_bedroom", Label: "2 phòng ngủ"},
	{Key: "duplex", Label: "Duplex / nhiều tầng"},
}

// hkRoomTypeFromBedrooms suy loại phòng từ số phòng ngủ của listing Dayladau.
// API trả 0 cho căn không chia phòng; từ 3 phòng ngủ trở lên coi như duplex vì
// khối lượng dọn tương đương, không phải vì kiến trúc giống nhau.
func hkRoomTypeFromBedrooms(bedrooms int) string {
	switch {
	case bedrooms <= 0:
		return "studio"
	case bedrooms == 1:
		return "one_bedroom"
	case bedrooms == 2:
		return "two_bedroom"
	default:
		return "duplex"
	}
}

// ─── Tiến độ checklist ────────────────────────────────────────────────────

type HKProgress struct {
	TotalRequired int      `json:"total_required"`
	DoneRequired  int      `json:"done_required"`
	TotalAll      int      `json:"total_all"`
	DoneAll       int      `json:"done_all"`
	PhotoCount    int      `json:"photo_count"`
	Percent       int      `json:"percent"`
	Complete      bool     `json:"complete"`
	Missing       []string `json:"missing"`
}

func hkMinPhotos(it HKItem) int {
	if it.MinPhotos <= 0 {
		return defaultMinPhotos
	}
	return it.MinPhotos
}

func hkFlattenItems(t *HKTemplate) []HKItem {
	if t == nil {
		return nil
	}
	var out []HKItem
	for _, g := range t.Groups {
		out = append(out, g.Items...)
	}
	return out
}

func hkPhotosOf(s *HKSession, itemID string) []HKPhoto {
	if s == nil || s.ItemsState == nil {
		return nil
	}
	st := s.ItemsState[itemID]
	out := make([]HKPhoto, 0, len(st.Photos))
	for _, p := range st.Photos {
		if strings.TrimSpace(p.URL) != "" {
			out = append(out, p)
		}
	}
	return out
}

func hkItemDone(s *HKSession, it HKItem) bool {
	if !it.RequirePhoto {
		if s == nil || s.ItemsState == nil {
			return false
		}
		return s.ItemsState[it.ID].Checked
	}
	return len(hkPhotosOf(s, it.ID)) >= hkMinPhotos(it)
}

// hkSessionProgress tính tiến độ từ mẫu checklist.
//
// Chỉ mục RequirePhoto mới chặn nộp. Mục không yêu cầu ảnh (VD "báo chủ nhà nếu
// thiếu đồ") vẫn hiện để cô tick nhưng không giữ công lại — chặn tiền vì một cái
// tick không có bằng chứng thì chỉ dạy người ta tick bừa.
func hkSessionProgress(s *HKSession, t *HKTemplate) HKProgress {
	items := hkFlattenItems(t)
	p := HKProgress{TotalAll: len(items), Missing: []string{}}
	for _, it := range items {
		done := hkItemDone(s, it)
		if done {
			p.DoneAll++
		}
		p.PhotoCount += len(hkPhotosOf(s, it.ID))
		if !it.RequirePhoto {
			continue
		}
		p.TotalRequired++
		if done {
			p.DoneRequired++
		} else {
			p.Missing = append(p.Missing, it.Title)
		}
	}
	// Phần trăm tính trên mục BẮT BUỘC vì đó là thứ chặn nộp. Tính trên tất cả sẽ
	// hiện 90% trong khi vẫn còn một ảnh bắt buộc thiếu — thanh gần đầy mà bấm nộp
	// không được là kiểu bực bội vô ích nhất.
	switch {
	case p.TotalRequired > 0:
		p.Percent = p.DoneRequired * 100 / p.TotalRequired
	case p.DoneAll > 0:
		p.Percent = 100
	}
	p.Complete = p.TotalRequired > 0 && p.DoneRequired == p.TotalRequired
	return p
}

// hkTemplateFor trả mẫu dùng để chấm ca: ưu tiên bản chụp lúc tạo ca.
func hkTemplateFor(s *HKSession, live *HKTemplate) *HKTemplate {
	if s != nil && s.TemplateSnapshot != nil && len(s.TemplateSnapshot.Groups) > 0 {
		return s.TemplateSnapshot
	}
	return live
}

// ─── Chuyển trạng thái ────────────────────────────────────────────────────

// hkDeriveStatus suy trạng thái TỪ DỮ LIỆU, không tin cờ tự do.
//
// Quyết định của quản lý (duyệt/từ chối) giữ nguyên. Còn lại thì đủ ảnh =
// submitted, có động tĩnh = in_progress, chưa gì = todo. Nhờ vậy không có cảnh ca
// đủ ảnh mà vẫn nằm ở "đang dọn" chỉ vì lỡ mất một lần ghi.
func hkDeriveStatus(s *HKSession, t *HKTemplate) string {
	if s.Status == HKSessionApproved || s.Status == HKSessionRejected {
		return s.Status
	}
	p := hkSessionProgress(s, t)
	switch {
	case p.Complete:
		return HKSessionSubmitted
	case p.PhotoCount > 0 || p.DoneAll > 0 || s.StartedAt > 0:
		return HKSessionInProgress
	default:
		return HKSessionTodo
	}
}

// hkNormalizePhone bỏ mọi ký tự không phải số để "0912 345 601", "0912345601" và
// "+84912345601" không thành ba tài khoản khác nhau.
func hkNormalizePhone(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	// 84xxxxxxxxx → 0xxxxxxxxx. API Dayladau trả số dạng 84…, người dùng gõ 0…
	if strings.HasPrefix(s, "84") && len(s) >= 10 {
		s = "0" + s[2:]
	}
	return s
}

// ─── Chỉ số hiệu suất ─────────────────────────────────────────────────────
//
// Hệ thống này KHÔNG tính lương. Lương của cô dọn dẹp tính theo cơ chế riêng
// (lương cứng + thưởng review + thưởng ngoài) ở ngoài phần mềm. Ở đây chỉ ghi
// nhận công việc đã làm và đo hiệu suất, để quản lý có số liệu và để chính cô
// thấy mình đang làm thế nào.

type HKPerfRow struct {
	StaffID string `json:"staff_id"`
	Name    string `json:"name"`
	Phone   string `json:"phone"`

	Sessions  int `json:"sessions"`   // số ca dọn đã hoàn tất
	Rooms     int `json:"rooms"`      // số phòng khác nhau đã dọn
	Approved  int `json:"approved"`   // quản lý đã duyệt ảnh
	Pending   int `json:"pending"`    // đủ ảnh, chờ quản lý xem
	Rejected  int `json:"rejected"`   // bị trả lại
	Late      int `json:"late"`       // xong sau hạn
	Photos    int `json:"photos"`     // tổng ảnh đã chụp
	AvgMinute int `json:"avg_minute"` // thời gian dọn trung bình, phút
}

// hkSessionMinutes — thời gian dọn một ca, tính từ lúc bấm bắt đầu tới lúc đủ ảnh.
//
// Trả 0 khi thiếu mốc hoặc số vô lý. Ca kéo dài quá 8 tiếng gần như chắc chắn là
// cô quên bấm bắt đầu từ hôm trước chứ không phải dọn 8 tiếng thật; đưa vào trung
// bình thì một bản ghi hỏng kéo lệch cả báo cáo tháng.
func hkSessionMinutes(s *HKSession) int {
	if s == nil || s.StartedAt <= 0 || s.SubmittedAt <= 0 {
		return 0
	}
	d := s.SubmittedAt - s.StartedAt
	if d <= 0 || d > 8*3600*1000 {
		return 0
	}
	return int(d / 60000)
}

// hkBuildPerf gom hiệu suất theo người trong khoảng thời gian đã lọc.
func hkBuildPerf(sessions []HKSession, users map[string]HKUser, progressOf func(*HKSession) HKProgress) []HKPerfRow {
	type acc struct {
		row     *HKPerfRow
		rooms   map[string]bool
		minutes []int
	}
	byStaff := map[string]*acc{}
	var order []string

	for i := range sessions {
		s := &sessions[i]
		if s.StaffID == "" {
			continue
		}
		a, ok := byStaff[s.StaffID]
		if !ok {
			u := users[s.StaffID]
			name := u.Name
			if name == "" {
				name = "Không rõ"
			}
			a = &acc{row: &HKPerfRow{StaffID: s.StaffID, Name: name, Phone: u.Phone}, rooms: map[string]bool{}}
			byStaff[s.StaffID] = a
			order = append(order, s.StaffID)
		}

		switch s.Status {
		case HKSessionApproved:
			a.row.Approved++
		case HKSessionSubmitted:
			a.row.Pending++
		case HKSessionRejected:
			a.row.Rejected++
		}
		// Chỉ đếm là "đã dọn" khi thật sự đủ ảnh — ca bỏ dở không phải thành tích.
		if s.Status != HKSessionApproved && s.Status != HKSessionSubmitted {
			continue
		}
		a.row.Sessions++
		a.rooms[s.RoomID] = true
		if progressOf != nil {
			a.row.Photos += progressOf(s).PhotoCount
		}
		if m := hkSessionMinutes(s); m > 0 {
			a.minutes = append(a.minutes, m)
		}
		if s.DeadlineAt > 0 && s.SubmittedAt > s.DeadlineAt {
			a.row.Late++
		}
	}

	out := make([]HKPerfRow, 0, len(order))
	for _, id := range order {
		a := byStaff[id]
		a.row.Rooms = len(a.rooms)
		if n := len(a.minutes); n > 0 {
			sum := 0
			for _, m := range a.minutes {
				sum += m
			}
			a.row.AvgMinute = sum / n
		}
		out = append(out, *a.row)
	}
	// Nhiều ca nhất lên đầu; bằng nhau thì theo tên để thứ tự ổn định.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].Sessions > out[j-1].Sessions ||
			(out[j].Sessions == out[j-1].Sessions && out[j].Name < out[j-1].Name)); j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
