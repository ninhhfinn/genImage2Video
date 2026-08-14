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
	HKAllowancePending  = "pending"
	HKAllowanceApproved = "approved"
	HKAllowanceRejected = "rejected"
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
	BaseFee    int64  `json:"base_fee"`
	CheckinHr  int    `json:"checkin_hour"`
	CheckoutHr int    `json:"checkout_hour"`
	Active     bool   `json:"active"`
	SyncedAt   int64  `json:"synced_at"`
}

type HKItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	RequirePhoto bool   `json:"require_photo"`
	MinPhotos    int    `json:"min_photos,omitempty"`
	Hint         string `json:"hint,omitempty"`
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

type HKAllowance struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Type      string    `json:"type"`
	Amount    int64     `json:"amount"`
	Note      string    `json:"note"`
	Photos    []HKPhoto `json:"photos"`
	Status    string    `json:"status"`
	CreatedAt int64     `json:"created_at"`
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
	BaseFee   int64  `json:"base_fee"`
	Deduction int64  `json:"deduction"`

	ItemsState map[string]HKItemState `json:"items_state"`
	Allowances []HKAllowance          `json:"allowances"`

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

// ─── Loại phòng & đơn giá khoán ───────────────────────────────────────────

type HKRoomType struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	BaseFee int64  `json:"base_fee"`
}

// Đơn giá khoán mặc định. Để trong code vì đây là bản đầu; khi giá đổi theo mùa
// (Tết) thì chuyển sang bảng cấu hình để không phải deploy lại.
var hkRoomTypes = []HKRoomType{
	{Key: "studio", Label: "Studio", BaseFee: 80000},
	{Key: "one_bedroom", Label: "1 phòng ngủ", BaseFee: 100000},
	{Key: "two_bedroom", Label: "2 phòng ngủ", BaseFee: 140000},
	{Key: "duplex", Label: "Duplex / nhiều tầng", BaseFee: 180000},
}

func hkDefaultBaseFee(roomType string) int64 {
	for _, t := range hkRoomTypes {
		if t.Key == roomType {
			return t.BaseFee
		}
	}
	return 0
}

// hkRoomTypeFromBedrooms suy loại phòng từ số phòng ngủ của listing Dayladau.
// 0 phòng ngủ = studio (API trả 0 cho căn không chia phòng), >=3 coi như duplex
// vì khối lượng dọn tương đương chứ không phải vì kiến trúc giống nhau.
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

// ─── Loại phụ cấp ─────────────────────────────────────────────────────────

type HKAllowanceType struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	DefaultAmount int64  `json:"default_amount"`
}

var hkAllowanceTypes = []HKAllowanceType{
	{Key: "bed_linen", Label: "Giặt chăn ga gối", DefaultAmount: 30000},
	{Key: "deep_clean", Label: "Dọn sâu (khách để bẩn)", DefaultAmount: 50000},
	{Key: "extra_trash", Label: "Rác phát sinh nhiều", DefaultAmount: 20000},
	{Key: "far_travel", Label: "Di chuyển xa", DefaultAmount: 25000},
	{Key: "supply_buy", Label: "Mua đồ tiêu hao hộ", DefaultAmount: 0},
	{Key: "other", Label: "Khác", DefaultAmount: 0},
}

func hkValidAllowanceType(key string) bool {
	for _, t := range hkAllowanceTypes {
		if t.Key == key {
			return true
		}
	}
	return false
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

// ─── Tính công ────────────────────────────────────────────────────────────

type HKPay struct {
	Base             int64 `json:"base"`
	Deduction        int64 `json:"deduction"`
	Allowance        int64 `json:"allowance"`
	AllowancePending int64 `json:"allowance_pending"`
	Total            int64 `json:"total"`
	Payable          bool  `json:"payable"`
	Confirmed        bool  `json:"confirmed"`
}

// hkPayable: ca `submitted` VẪN tính tiền (cột tạm tính).
//
// Cô làm xong việc thì phải thấy tiền của mình ngay; nếu chỉ đếm ca đã duyệt thì
// suốt tuần bảng công hiện 0đ và không ai tin cái tool.
func hkPayable(status string) bool {
	return status == HKSessionApproved || status == HKSessionSubmitted
}

func hkSumAllowances(s *HKSession, status string) int64 {
	var sum int64
	for _, a := range s.Allowances {
		st := a.Status
		if st == "" {
			st = HKAllowancePending
		}
		if st == status {
			sum += a.Amount
		}
	}
	return sum
}

// hkComputePay: công = khoán phòng − trừ + phụ cấp ĐÃ DUYỆT.
//
// Phụ cấp `pending` không cộng vào bất kỳ cột nào, kể cả tạm tính: nó là khoản cô
// tự khai, hiện như tiền đã có rồi sau đó quản lý cắt đi mới là thứ gây cãi nhau
// thật sự. Nó nằm riêng ở AllowancePending để cả hai bên thấy "đang chờ duyệt".
func hkComputePay(s *HKSession) HKPay {
	if s == nil {
		return HKPay{}
	}
	p := HKPay{
		Payable:          hkPayable(s.Status),
		Confirmed:        s.Status == HKSessionApproved,
		AllowancePending: hkSumAllowances(s, HKAllowancePending),
	}
	if !p.Payable {
		return p
	}
	p.Base = s.BaseFee
	// Trừ không được vượt quá tiền khoán — công âm là lỗi nhập liệu, không phải
	// chính sách. Trừ âm (nhập sai dấu) cũng bị bỏ qua, không biến thành thưởng.
	d := s.Deduction
	if d < 0 {
		d = 0
	}
	if d > p.Base {
		d = p.Base
	}
	p.Deduction = d
	p.Allowance = hkSumAllowances(s, HKAllowanceApproved)
	p.Total = p.Base - p.Deduction + p.Allowance
	return p
}

// ─── Bảng công ────────────────────────────────────────────────────────────

type HKTimesheetRow struct {
	StaffID          string `json:"staff_id"`
	Name             string `json:"name"`
	Phone            string `json:"phone"`
	Bank             string `json:"bank"`
	Rooms            int    `json:"rooms"`
	RoomsConfirmed   int    `json:"rooms_confirmed"`
	RoomsPending     int    `json:"rooms_pending"`
	Rejected         int    `json:"rejected"`
	Base             int64  `json:"base"`
	Deduction        int64  `json:"deduction"`
	Allowance        int64  `json:"allowance"`
	AllowancePending int64  `json:"allowance_pending"`
	ConfirmedTotal   int64  `json:"confirmed_total"`
	ProvisionalTotal int64  `json:"provisional_total"`
	Total            int64  `json:"total"`
}

func hkBuildTimesheet(sessions []HKSession, users map[string]HKUser) []HKTimesheetRow {
	byStaff := map[string]*HKTimesheetRow{}
	var order []string

	for i := range sessions {
		s := &sessions[i]
		if s.StaffID == "" {
			continue
		}
		row, ok := byStaff[s.StaffID]
		if !ok {
			u := users[s.StaffID]
			name := u.Name
			if name == "" {
				// Ca trỏ tới tài khoản đã xoá vẫn phải hiện — nuốt mất nó là nuốt
				// mất tiền của một người đã làm việc thật.
				name = "Không rõ"
			}
			row = &HKTimesheetRow{StaffID: s.StaffID, Name: name, Phone: u.Phone, Bank: u.Bank}
			byStaff[s.StaffID] = row
			order = append(order, s.StaffID)
		}

		if s.Status == HKSessionRejected {
			row.Rejected++
		}
		pay := hkComputePay(s)
		row.AllowancePending += pay.AllowancePending
		if !pay.Payable {
			continue
		}
		row.Rooms++
		row.Base += pay.Base
		row.Deduction += pay.Deduction
		row.Allowance += pay.Allowance
		row.Total += pay.Total
		if pay.Confirmed {
			row.RoomsConfirmed++
			row.ConfirmedTotal += pay.Total
		} else {
			row.RoomsPending++
			row.ProvisionalTotal += pay.Total
		}
	}

	out := make([]HKTimesheetRow, 0, len(order))
	for _, id := range order {
		out = append(out, *byStaff[id])
	}
	// Sắp theo tổng tiền giảm dần; bằng nhau thì theo tên để thứ tự ổn định giữa
	// các lần gọi (bảng công nhảy lung tung giữa hai lần F5 làm mất tin tưởng).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			a, b := out[j-1], out[j]
			if b.Total > a.Total || (b.Total == a.Total && b.Name < a.Name) {
				out[j-1], out[j] = b, a
				continue
			}
			break
		}
	}
	return out
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
