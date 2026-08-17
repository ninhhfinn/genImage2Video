package main

// Vấn đề phát sinh tại phòng → danh sách việc cần xử lý.
//
// Luồng: cô dọn dẹp phát hiện hỏng hóc/thiếu đồ khi dọn → báo kèm ảnh → việc
// vào danh sách chung → kỹ thuật tự nhận hoặc quản lý giao → xử lý xong chụp ảnh
// đóng việc.
//
// VÌ SAO TÁCH KHỎI CHECKLIST: checklist là những việc PHẢI làm mọi ca, còn đây
// là việc CÓ THỂ phát sinh. Nhét vào checklist thì cô phải đi qua nó mỗi ca chỉ
// để bấm "không có gì" — thêm một chạm vô ích cho phần lớn số ca.

import (
	"strings"
	"time"
)

// ─── Trạng thái & mức độ ──────────────────────────────────────────────────

const (
	HKIssueOpen     = "open"     // đã báo, chưa ai nhận
	HKIssueAssigned = "assigned" // đã có người phụ trách
	HKIssueDone     = "done"     // đã xử lý xong
	HKIssueRejected = "rejected" // không phải vấn đề / bỏ qua
)

const (
	HKUrgencyUrgent = "urgent" // chặn khách vào — phải xong trong ngày
	HKUrgencyNormal = "normal"
)

var hkIssueStatusLabel = map[string]string{
	HKIssueOpen:     "Chờ nhận",
	HKIssueAssigned: "Đang xử lý",
	HKIssueDone:     "Đã xong",
	HKIssueRejected: "Bỏ qua",
}

// HKRoleHandler — nhân sự kỹ thuật xử lý sự cố. Vai thứ ba bên cạnh quản lý và
// cô dọn dẹp: họ thấy danh sách vấn đề để tự nhận việc, không thấy ca dọn hay
// doanh thu.
const HKRoleHandler = "handler"

// ─── Loại vấn đề ──────────────────────────────────────────────────────────

type HKIssueCategory struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Loại nào thường chặn khách vào thì gợi ý sẵn mức khẩn, cô không phải tự
	// đánh giá — điện nước hỏng lúc 11h trưa mà để "bình thường" là mất khách.
	SuggestUrgent bool `json:"suggest_urgent"`
}

var hkIssueCategories = []HKIssueCategory{
	{Key: "dien_nuoc", Label: "Điện, nước, điều hoà", SuggestUrgent: true},
	{Key: "khoa_cua", Label: "Khoá, cửa, thẻ từ", SuggestUrgent: true},
	{Key: "thiet_bi", Label: "Thiết bị hỏng (tivi, máy giặt, bình nóng lạnh)"},
	{Key: "thieu_do", Label: "Thiếu đồ (khăn, chăn ga, đồ dùng)"},
	{Key: "hu_hong", Label: "Hư hỏng nội thất, tường, sàn"},
	{Key: "ve_sinh", Label: "Vệ sinh cần xử lý sâu (mốc, côn trùng, mùi)"},
	{Key: "khac", Label: "Khác"},
}

func hkValidIssueCategory(key string) bool {
	for _, c := range hkIssueCategories {
		if c.Key == key {
			return true
		}
	}
	return false
}

func hkCategoryLabel(key string) string {
	for _, c := range hkIssueCategories {
		if c.Key == key {
			return c.Label
		}
	}
	return key
}

// ─── Kiểu dữ liệu ─────────────────────────────────────────────────────────

type HKIssue struct {
	ID        string `json:"id"`
	RoomID    string `json:"room_id"`
	SessionID string `json:"session_id"` // ca dọn phát hiện ra, có thể rỗng

	ReporterID   string `json:"reporter_id"`
	ReporterName string `json:"reporter_name"`

	Category    string    `json:"category"`
	Urgency     string    `json:"urgency"`
	Description string    `json:"description"`
	Photos      []HKPhoto `json:"photos"`

	Status     string `json:"status"`
	AssigneeID string `json:"assignee_id"`
	DeadlineAt int64  `json:"deadline_at"`

	ResolveNote   string    `json:"resolve_note"`
	ResolvePhotos []HKPhoto `json:"resolve_photos"`

	CreatedAt  int64 `json:"created_at"`
	AssignedAt int64 `json:"assigned_at"`
	ResolvedAt int64 `json:"resolved_at"`
}

// ─── Luật nghiệp vụ ───────────────────────────────────────────────────────

// hkSuggestDeadline gợi ý hạn xử lý theo mức độ.
//
// Khẩn → cuối ngày hôm nay: những thứ chặn khách vào (điện, nước, khoá) mà để
// sang mai thì khách đã đến rồi. Bình thường → 3 ngày, đủ để gom việc lại làm
// một lượt thay vì chạy đi chạy lại từng căn.
//
// Chỉ là GỢI Ý — người nhận việc hoặc quản lý sửa lại được.
func hkSuggestDeadline(urgency string, now time.Time) int64 {
	loc := now.Location()
	if urgency == HKUrgencyUrgent {
		end := time.Date(now.Year(), now.Month(), now.Day(), 21, 0, 0, 0, loc)
		if now.After(end) {
			// Báo sau 21h thì hạn là sáng hôm sau, không phải một mốc đã trôi qua.
			end = time.Date(now.Year(), now.Month(), now.Day()+1, 12, 0, 0, 0, loc)
		}
		return end.UnixMilli()
	}
	d := now.AddDate(0, 0, 3)
	return time.Date(d.Year(), d.Month(), d.Day(), 18, 0, 0, 0, loc).UnixMilli()
}

// hkIssueOverdue — quá hạn chưa. Việc đã xong hoặc bỏ qua thì không tính:
// đóng muộn vẫn là đóng, gắn cờ đỏ vĩnh viễn chỉ làm nhiễu danh sách.
func hkIssueOverdue(i *HKIssue, now int64) bool {
	if i == nil || i.DeadlineAt <= 0 {
		return false
	}
	if i.Status == HKIssueDone || i.Status == HKIssueRejected {
		return false
	}
	return now > i.DeadlineAt
}

// hkIssueOpenForClaim — việc còn nhận được không.
func hkIssueOpenForClaim(i *HKIssue) bool {
	return i != nil && i.Status == HKIssueOpen
}

// ─── Tổng hợp cho báo cáo hằng ngày ───────────────────────────────────────

type HKIssueSummary struct {
	Total     int `json:"total"`
	Open      int `json:"open"`      // chưa ai nhận
	Assigned  int `json:"assigned"`  // đang xử lý
	Done      int `json:"done"`      // đã xong
	Overdue   int `json:"overdue"`   // quá hạn, chưa xong
	Urgent    int `json:"urgent"`    // mức khẩn, chưa xong
	NewToday  int `json:"new_today"` // báo mới hôm nay
	DoneToday int `json:"done_today"`
}

func hkSummarizeIssues(list []HKIssue, now time.Time) HKIssueSummary {
	var s HKIssueSummary
	nowMs := now.UnixMilli()
	today := now.Format("2006-01-02")
	loc := now.Location()
	dayOf := func(ms int64) string {
		if ms <= 0 {
			return ""
		}
		return time.UnixMilli(ms).In(loc).Format("2006-01-02")
	}

	for i := range list {
		it := &list[i]
		s.Total++
		switch it.Status {
		case HKIssueOpen:
			s.Open++
		case HKIssueAssigned:
			s.Assigned++
		case HKIssueDone:
			s.Done++
		}
		if hkIssueOverdue(it, nowMs) {
			s.Overdue++
		}
		// Khẩn CHƯA xong mới đáng báo động; khẩn đã xử lý xong là chuyện tốt.
		if it.Urgency == HKUrgencyUrgent && it.Status != HKIssueDone && it.Status != HKIssueRejected {
			s.Urgent++
		}
		if dayOf(it.CreatedAt) == today {
			s.NewToday++
		}
		if dayOf(it.ResolvedAt) == today {
			s.DoneToday++
		}
	}
	return s
}

// hkSortIssues sắp theo mức độ cấp thiết: quá hạn trước, rồi khẩn, rồi hạn gần
// nhất. Việc đã đóng xuống cuối.
//
// Người mở danh sách ra là để biết "phải làm gì trước", không phải để đọc theo
// thứ tự thời gian báo.
func hkSortIssues(list []HKIssue, now int64) {
	rank := func(i *HKIssue) int {
		switch {
		case i.Status == HKIssueDone || i.Status == HKIssueRejected:
			return 4
		case hkIssueOverdue(i, now):
			return 0
		case i.Urgency == HKUrgencyUrgent:
			return 1
		case i.Status == HKIssueOpen:
			return 2
		default:
			return 3
		}
	}
	for i := 1; i < len(list); i++ {
		for j := i; j > 0; j-- {
			a, b := &list[j-1], &list[j]
			ra, rb := rank(a), rank(b)
			if rb < ra || (rb == ra && hkDeadlineKey(b) < hkDeadlineKey(a)) {
				list[j-1], list[j] = list[j], list[j-1]
				continue
			}
			break
		}
	}
}

// hkDeadlineKey — việc chưa có hạn xếp sau việc có hạn, thay vì lên đầu vì số 0.
func hkDeadlineKey(i *HKIssue) int64 {
	if i.DeadlineAt <= 0 {
		return 1<<62 - 1
	}
	return i.DeadlineAt
}

func hkTrimIssueText(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > max {
		return string(r[:max])
	}
	return s
}
