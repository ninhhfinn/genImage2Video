package main

// HTTP cho luồng vấn đề → việc cần xử lý.
//
// Ba vai nhìn thấy ba thứ khác nhau:
//   • Cô dọn dẹp — báo vấn đề, và xem lại vấn đề CHÍNH MÌNH đã báo (để biết đã
//     được xử lý chưa; báo xong rơi vào im lặng thì lần sau không ai báo nữa).
//   • Kỹ thuật — xem mọi việc còn mở để tự nhận, và việc của mình.
//   • Quản lý — xem tất cả, giao việc, đổi hạn, đóng việc.

import (
	"net/http"
	"strings"
	"time"
)

type HKIssueView struct {
	HKIssue
	RoomCode     string `json:"room_code"`
	RoomName     string `json:"room_name"`
	Facility     string `json:"facility_label"`
	CategoryText string `json:"category_text"`
	StatusText   string `json:"status_text"`
	AssigneeName string `json:"assignee_name"`
	Overdue      bool   `json:"overdue"`
}

func (a *HKApp) hkBuildIssueViews(list []HKIssue) ([]HKIssueView, error) {
	rooms, err := a.store.ListRooms(false)
	if err != nil {
		return nil, err
	}
	roomByID := map[string]HKRoom{}
	for _, r := range rooms {
		roomByID[r.ID] = r
	}
	users, err := a.store.ListUsers("")
	if err != nil {
		return nil, err
	}
	nameOf := map[string]string{}
	for _, u := range users {
		nameOf[u.ID] = u.Name
	}

	now := hkNowMs()
	out := make([]HKIssueView, 0, len(list))
	for i := range list {
		it := list[i]
		room := roomByID[it.RoomID]
		it.ReporterName = nameOf[it.ReporterID]
		out = append(out, HKIssueView{
			HKIssue:      it,
			RoomCode:     room.Code,
			RoomName:     room.Name,
			Facility:     room.FacilityLabel,
			CategoryText: hkCategoryLabel(it.Category),
			StatusText:   hkIssueStatusLabel[it.Status],
			AssigneeName: nameOf[it.AssigneeID],
			Overdue:      hkIssueOverdue(&it, now),
		})
	}
	return out, nil
}

// ─── Báo vấn đề ───────────────────────────────────────────────────────────

func (a *HKApp) handleIssueCreate(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	u, err := a.hkAuthUser(r)
	if err != nil {
		hkFailAuth(w, err)
		return
	}
	var body struct {
		RoomID      string    `json:"room_id"`
		SessionID   string    `json:"session_id"`
		Category    string    `json:"category"`
		Urgency     string    `json:"urgency"`
		Description string    `json:"description"`
		Photos      []HKPhoto `json:"photos"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	if strings.TrimSpace(body.RoomID) == "" {
		hkFail(w, http.StatusBadRequest, "Chưa chọn phòng.")
		return
	}
	if _, err := a.store.RoomByID(body.RoomID); err != nil {
		hkFail(w, http.StatusNotFound, "Không tìm thấy phòng này.")
		return
	}
	if !hkValidIssueCategory(body.Category) {
		hkFail(w, http.StatusBadRequest, "Chưa chọn loại vấn đề.")
		return
	}
	desc := hkTrimIssueText(body.Description, 1000)
	if desc == "" {
		hkFail(w, http.StatusBadRequest, "Mô tả giúp người sửa biết phải mang theo gì — viết vài chữ nhé.")
		return
	}
	urgency := HKUrgencyNormal
	if body.Urgency == HKUrgencyUrgent {
		urgency = HKUrgencyUrgent
	}

	// Chỉ nhận ảnh do chính server cấp phát, cùng lý do như ảnh checklist.
	photos := []HKPhoto{}
	for _, p := range body.Photos {
		if strings.HasPrefix(p.URL, "/api/hk/photo/") {
			photos = append(photos, p)
		}
	}

	now := time.Now()
	issue := HKIssue{
		ID:          hkRandomID("hkis"),
		RoomID:      body.RoomID,
		SessionID:   strings.TrimSpace(body.SessionID),
		ReporterID:  u.ID,
		Category:    body.Category,
		Urgency:     urgency,
		Description: desc,
		Photos:      photos,
		Status:      HKIssueOpen,
		DeadlineAt:  hkSuggestDeadline(urgency, now),
		CreatedAt:   now.UnixMilli(),
	}
	if err := a.store.InsertIssue(issue); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được báo cáo.")
		return
	}
	a.writeIssue(w, issue.ID)
}

func (a *HKApp) writeIssue(w http.ResponseWriter, id string) {
	issue, err := a.store.IssueByID(id)
	if err != nil {
		hkFail(w, http.StatusNotFound, "Không tìm thấy vấn đề này.")
		return
	}
	views, err := a.hkBuildIssueViews([]HKIssue{issue})
	if err != nil || len(views) == 0 {
		hkFail(w, http.StatusInternalServerError, "Không dựng được dữ liệu.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"issue": views[0]})
}

// ─── Danh sách ────────────────────────────────────────────────────────────

func (a *HKApp) handleIssues(w http.ResponseWriter, r *http.Request) {
	u, err := a.hkAuthUser(r)
	if err != nil {
		hkFailAuth(w, err)
		return
	}
	q := r.URL.Query()
	f := HKIssueFilter{
		Status:   strings.TrimSpace(q.Get("status")),
		RoomID:   strings.TrimSpace(q.Get("room_id")),
		Urgency:  strings.TrimSpace(q.Get("urgency")),
		OpenOnly: q.Get("open") == "1",
	}
	if v := strings.TrimSpace(q.Get("assignee_id")); v != "" {
		f.AssigneeID = v
	}

	switch u.Role {
	case HKRoleAdmin:
		// thấy hết
	case HKRoleHandler:
		// Kỹ thuật thấy mọi việc để tự nhận — đó là mục đích của vai này.
		// `mine=1` để xem riêng việc của mình.
		if q.Get("mine") == "1" {
			f.AssigneeID = u.ID
		}
	default:
		// Cô dọn dẹp chỉ thấy vấn đề CHÍNH MÌNH đã báo. Báo xong rơi vào im lặng
		// thì lần sau không ai buồn báo nữa.
		f.ReporterID = u.ID
	}

	list, err := a.store.ListIssues(f)
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không đọc được danh sách vấn đề.")
		return
	}
	hkSortIssues(list, hkNowMs())

	views, err := a.hkBuildIssueViews(list)
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không dựng được danh sách.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{
		"issues":     views,
		"summary":    hkSummarizeIssues(list, time.Now()),
		"categories": hkIssueCategories,
	})
}

// ─── Nhận việc / giao việc ────────────────────────────────────────────────

// handleIssueClaim — kỹ thuật tự nhận việc.
func (a *HKApp) handleIssueClaim(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	u, err := a.hkAuthUser(r)
	if err != nil {
		hkFailAuth(w, err)
		return
	}
	if u.Role != HKRoleHandler && u.Role != HKRoleAdmin {
		hkFail(w, http.StatusForbidden, "Chỉ nhân sự kỹ thuật mới nhận việc được.")
		return
	}
	var body struct {
		ID         string `json:"id"`
		DeadlineAt int64  `json:"deadline_at"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	issue, err := a.store.IssueByID(body.ID)
	if err != nil {
		hkFail(w, http.StatusNotFound, "Không tìm thấy vấn đề này.")
		return
	}
	// Ai nhận trước được trước. Không có việc hai người cùng đi sửa một chỗ rồi
	// mới phát hiện ra nhau.
	if !hkIssueOpenForClaim(&issue) {
		if issue.AssigneeID == u.ID {
			a.writeIssue(w, issue.ID)
			return
		}
		hkFail(w, http.StatusConflict, "Việc này đã có người nhận rồi.")
		return
	}

	issue.AssigneeID = u.ID
	issue.Status = HKIssueAssigned
	issue.AssignedAt = hkNowMs()
	if body.DeadlineAt > 0 {
		issue.DeadlineAt = body.DeadlineAt
	}
	if err := a.store.UpdateIssue(issue); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được.")
		return
	}
	a.writeIssue(w, issue.ID)
}

// handleIssueAssign — quản lý giao việc cho người khác.
func (a *HKApp) handleIssueAssign(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	if _, err := a.hkRequireAdmin(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	var body struct {
		ID         string `json:"id"`
		AssigneeID string `json:"assignee_id"`
		DeadlineAt int64  `json:"deadline_at"`
		Urgency    string `json:"urgency"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	issue, err := a.store.IssueByID(body.ID)
	if err != nil {
		hkFail(w, http.StatusNotFound, "Không tìm thấy vấn đề này.")
		return
	}
	if body.AssigneeID != "" {
		target, err := a.store.UserByID(body.AssigneeID)
		if err != nil || target.Status != HKStaffActive {
			hkFail(w, http.StatusBadRequest, "Chỉ giao được cho nhân sự đang làm việc.")
			return
		}
		issue.AssigneeID = target.ID
		issue.Status = HKIssueAssigned
		issue.AssignedAt = hkNowMs()
	} else {
		// Gỡ người phụ trách → việc quay lại hàng chờ, không biến mất.
		issue.AssigneeID = ""
		if issue.Status == HKIssueAssigned {
			issue.Status = HKIssueOpen
		}
	}
	if body.Urgency == HKUrgencyUrgent || body.Urgency == HKUrgencyNormal {
		issue.Urgency = body.Urgency
	}
	if body.DeadlineAt > 0 {
		issue.DeadlineAt = body.DeadlineAt
	}
	if err := a.store.UpdateIssue(issue); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được.")
		return
	}
	a.writeIssue(w, issue.ID)
}

// ─── Đóng việc ────────────────────────────────────────────────────────────

func (a *HKApp) handleIssueResolve(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	u, err := a.hkAuthUser(r)
	if err != nil {
		hkFailAuth(w, err)
		return
	}
	var body struct {
		ID     string    `json:"id"`
		Status string    `json:"status"`
		Note   string    `json:"note"`
		Photos []HKPhoto `json:"photos"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	if body.Status != HKIssueDone && body.Status != HKIssueRejected {
		hkFail(w, http.StatusBadRequest, "Chỉ đánh dấu xong hoặc bỏ qua được.")
		return
	}
	issue, err := a.store.IssueByID(body.ID)
	if err != nil {
		hkFail(w, http.StatusNotFound, "Không tìm thấy vấn đề này.")
		return
	}
	// Người đang phụ trách hoặc quản lý mới đóng được. Không cho người ngoài đóng
	// việc của người khác — đó là cách một việc chưa làm bị đánh dấu xong.
	if u.Role != HKRoleAdmin && issue.AssigneeID != u.ID {
		hkFail(w, http.StatusForbidden, "Chỉ người đang phụ trách hoặc quản lý mới đóng được việc này.")
		return
	}
	// "Bỏ qua" là quyết định quản lý, không phải của người ngại làm.
	if body.Status == HKIssueRejected && u.Role != HKRoleAdmin {
		hkFail(w, http.StatusForbidden, "Chỉ quản lý mới bỏ qua được một vấn đề.")
		return
	}

	photos := []HKPhoto{}
	for _, p := range body.Photos {
		if strings.HasPrefix(p.URL, "/api/hk/photo/") {
			photos = append(photos, p)
		}
	}
	issue.Status = body.Status
	issue.ResolveNote = hkTrimIssueText(body.Note, 1000)
	issue.ResolvePhotos = photos
	issue.ResolvedAt = hkNowMs()
	if err := a.store.UpdateIssue(issue); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được.")
		return
	}
	a.writeIssue(w, issue.ID)
}

// ─── Đổi vai nhân sự ──────────────────────────────────────────────────────

// handleStaffRole — quản lý đổi vai một tài khoản (cô dọn dẹp ↔ kỹ thuật).
func (a *HKApp) handleStaffRole(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	admin, err := a.hkRequireAdmin(r)
	if err != nil {
		hkFailAuth(w, err)
		return
	}
	var body struct {
		ID   string `json:"id"`
		Role string `json:"role"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	if body.Role != HKRoleCleaner && body.Role != HKRoleHandler {
		hkFail(w, http.StatusBadRequest, "Chỉ đổi được giữa Cô dọn dẹp và Kỹ thuật.")
		return
	}
	target, err := a.store.UserByID(body.ID)
	if err != nil {
		hkFail(w, http.StatusNotFound, "Không tìm thấy tài khoản.")
		return
	}
	if target.Role == HKRoleAdmin {
		hkFail(w, http.StatusForbidden, "Không đổi vai tài khoản quản lý ở màn này.")
		return
	}
	// Tự hạ vai chính mình thì mất quyền quản lý mà không lấy lại được.
	if target.ID == admin.ID {
		hkFail(w, http.StatusForbidden, "Không đổi vai của chính mình.")
		return
	}
	target.Role = body.Role
	if err := a.store.UpsertUser(target); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được.")
		return
	}
	updated, _ := a.store.UserByID(target.ID)
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"staff": updated})
}
