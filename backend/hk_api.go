package main

// HTTP handlers module Dọn dẹp.
//
// Một nguyên tắc xuyên suốt: **phép tính ra tiền chỉ tồn tại ở Go.** Backend trả
// về ca dọn kèm sẵn `progress` và `pay` đã tính; frontend chỉ hiển thị. Nếu để
// React tự cộng lại thì sớm muộn hai bên lệch nhau, và bên sai luôn là bên mà cô
// dọn dẹp nhìn thấy.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type HKApp struct {
	store    *HKStore
	photoDir string
}

func NewHKApp(dataDir string) (*HKApp, error) {
	store, err := OpenHKStore(filepath.Join(dataDir, "housekeeping.db"))
	if err != nil {
		return nil, err
	}
	photoDir := filepath.Join(dataDir, "photos")
	if err := os.MkdirAll(photoDir, 0755); err != nil {
		return nil, err
	}
	app := &HKApp{store: store, photoDir: photoDir}
	if err := hkSeedAdmin(store); err != nil {
		return nil, err
	}
	if err := app.hkSeedTemplates(); err != nil {
		return nil, err
	}
	store.PurgeExpiredTokens(hkNowMs())
	return app, nil
}

// ─── Tiện ích phản hồi ────────────────────────────────────────────────────

func hkWriteJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// hkFail trả lỗi kèm câu tiếng Việt cho NGƯỜI DÙNG CUỐI đọc, không phải mã lỗi.
// Cô dọn dẹp sẽ đọc thẳng chuỗi này trên điện thoại.
func hkFail(w http.ResponseWriter, code int, msg string) {
	hkWriteJSON(w, code, map[string]string{"error": msg})
}

func hkFailAuth(w http.ResponseWriter, err error) {
	if err == errHKForbidden {
		hkFail(w, http.StatusForbidden, "Tài khoản của bạn không có quyền làm việc này.")
		return
	}
	hkFail(w, http.StatusUnauthorized, "Phiên đăng nhập đã hết hạn. Đăng nhập lại nhé.")
}

func hkDecodeBody(r *http.Request, v interface{}) error {
	// Giới hạn 1MB: mọi body JSON ở đây đều nhỏ; ảnh đi đường multipart riêng.
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

func hkRequirePost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		hkFail(w, http.StatusMethodNotAllowed, "Sai phương thức.")
		return false
	}
	return true
}

// ─── Kiểu trả về cho giao diện ────────────────────────────────────────────

type HKSessionView struct {
	HKSession
	Room      HKRoom     `json:"room"`
	StaffName string     `json:"staff_name"`
	Progress  HKProgress `json:"progress"`
	Minutes   int        `json:"minutes"` // thời gian dọn, phút
	Late      bool       `json:"late"`
}

// hkBuildViews gắn phòng, người phụ trách, tiến độ, tiền vào mỗi ca.
//
// Trạng thái được suy lại từ dữ liệu (hkDeriveStatus) chứ không lấy nguyên cột
// trong DB: nếu một lần ghi bị rớt giữa chừng, ca đủ ảnh vẫn phải hiện là "chờ
// đối soát" thay vì kẹt ở "đang dọn" và giữ tiền của cô lại.
func (a *HKApp) hkBuildViews(sessions []HKSession) ([]HKSessionView, error) {
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
	userByID := map[string]HKUser{}
	for _, u := range users {
		userByID[u.ID] = u
	}
	templates, err := a.store.ListTemplates()
	if err != nil {
		return nil, err
	}
	tplByID := map[string]HKTemplate{}
	for _, t := range templates {
		tplByID[t.ID] = t
	}

	now := hkNowMs()
	out := make([]HKSessionView, 0, len(sessions))
	for i := range sessions {
		s := sessions[i]
		var live *HKTemplate
		if t, ok := tplByID[s.TemplateID]; ok {
			live = &t
		}
		tpl := hkTemplateFor(&s, live)
		s.Status = hkDeriveStatus(&s, tpl)

		v := HKSessionView{
			HKSession: s,
			Room:      roomByID[s.RoomID],
			StaffName: userByID[s.StaffID].Name,
			Progress:  hkSessionProgress(&s, tpl),
			Minutes:   hkSessionMinutes(&s),
		}
		// Chỉ gắn cờ trễ khi ca CHƯA đủ ảnh: nộp lúc 14:05 cho hạn 14:00 mà đã xong
		// việc thì không có gì để hối, cờ đỏ ở đó chỉ làm nhiễu bảng điều phối.
		v.Late = s.DeadlineAt > 0 && now > s.DeadlineAt &&
			s.Status != HKSessionSubmitted && s.Status != HKSessionApproved
		out = append(out, v)
	}
	return out, nil
}

// ─── Đăng nhập / đăng ký ──────────────────────────────────────────────────

func (a *HKApp) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	var body struct {
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}

	u, err := a.store.UserByPhone(body.Phone)
	// Sai số điện thoại và sai mật khẩu trả CÙNG một câu: nói rõ "số này chưa đăng
	// ký" là biến trang đăng nhập thành công cụ dò xem ai đang làm ở đây.
	if err != nil || !hkCheckPassword(u.PasswordHash, body.Password) {
		hkFail(w, http.StatusUnauthorized, "Số điện thoại hoặc mật khẩu chưa đúng.")
		return
	}

	switch u.Status {
	case HKStaffPending:
		hkFail(w, http.StatusForbidden, "Tài khoản đang chờ quản lý duyệt. Vui lòng liên hệ quản lý của bạn.")
		return
	case HKStaffSuspended:
		hkFail(w, http.StatusForbidden, "Tài khoản đang tạm khoá. Liên hệ quản lý để mở lại.")
		return
	case HKStaffRejected:
		hkFail(w, http.StatusForbidden, "Tài khoản không được duyệt.")
		return
	}

	token := hkNewToken()
	now := hkNowMs()
	if err := a.store.SaveToken(token, u.ID, now, now+hkTokenTTL.Milliseconds()); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không tạo được phiên đăng nhập, thử lại sau.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": u})
}

func (a *HKApp) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	var body struct {
		Name     string `json:"name"`
		Phone    string `json:"phone"`
		Password string `json:"password"`
		Zone     string `json:"zone"`
		Note     string `json:"note"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}

	name := strings.TrimSpace(body.Name)
	phone := hkNormalizePhone(body.Phone)
	if name == "" {
		hkFail(w, http.StatusBadRequest, "Bạn chưa nhập họ tên.")
		return
	}
	if len(phone) < 9 || len(phone) > 11 {
		hkFail(w, http.StatusBadRequest, "Số điện thoại chưa đúng. Ví dụ: 0912345678")
		return
	}
	if len(body.Password) < 6 {
		hkFail(w, http.StatusBadRequest, "Mật khẩu cần ít nhất 6 ký tự.")
		return
	}
	if _, err := a.store.UserByPhone(phone); err == nil {
		hkFail(w, http.StatusConflict, "Số điện thoại này đã đăng ký rồi. Bạn thử đăng nhập nhé.")
		return
	}

	hash, err := hkHashPassword(body.Password)
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không tạo được tài khoản, thử lại sau.")
		return
	}
	zones := []string{}
	if z := strings.TrimSpace(body.Zone); z != "" {
		zones = append(zones, z)
	}
	u := HKUser{
		ID:           hkRandomID("hku"),
		Role:         HKRoleCleaner,
		Name:         name,
		Phone:        phone,
		PasswordHash: hash,
		// Chờ duyệt: ai cũng mở được trang đăng ký vì nó nằm sau tên miền công khai.
		Status:    HKStaffPending,
		Zones:     zones,
		Note:      strings.TrimSpace(body.Note),
		CreatedAt: hkNowMs(),
	}
	if err := a.store.UpsertUser(u); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được đăng ký, thử lại sau.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"user": u})
}

func (a *HKApp) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := hkTokenFromRequest(r); token != "" {
		a.store.DeleteToken(token)
	}
	hkWriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *HKApp) handleMe(w http.ResponseWriter, r *http.Request) {
	u, err := a.hkAuthUser(r)
	if err != nil {
		hkFailAuth(w, err)
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"user": u})
}

// handleMeta phục vụ danh mục cho ô chọn: loại phòng + đơn giá, loại phụ cấp,
// khu vực. Frontend không tự chép lại các bảng này để hai bên không lệch giá.
func (a *HKApp) handleMeta(w http.ResponseWriter, r *http.Request) {
	rooms, _ := a.store.ListRooms(true)
	zoneSet := map[string]bool{}
	zones := []string{}
	for _, rm := range rooms {
		if rm.Zone != "" && !zoneSet[rm.Zone] {
			zoneSet[rm.Zone] = true
			zones = append(zones, rm.Zone)
		}
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"zones": zones})
}

// ─── Người dùng (quản lý) ─────────────────────────────────────────────────

func (a *HKApp) handleStaffs(w http.ResponseWriter, r *http.Request) {
	if _, err := a.hkRequireAdmin(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	users, err := a.store.ListUsers(HKRoleCleaner)
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không đọc được danh sách.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"staffs": users})
}

func (a *HKApp) handleStaffReview(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	if _, err := a.hkRequireAdmin(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	var body struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	switch body.Status {
	case HKStaffActive, HKStaffSuspended, HKStaffRejected, HKStaffPending:
	default:
		hkFail(w, http.StatusBadRequest, "Trạng thái không hợp lệ.")
		return
	}
	target, err := a.store.UserByID(body.ID)
	if err != nil {
		hkFail(w, http.StatusNotFound, "Không tìm thấy tài khoản.")
		return
	}
	if target.Role == HKRoleAdmin {
		hkFail(w, http.StatusForbidden, "Không đổi được trạng thái tài khoản quản lý ở màn này.")
		return
	}
	if err := a.store.SetUserStatus(body.ID, body.Status, hkNowMs()); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được thay đổi.")
		return
	}
	updated, _ := a.store.UserByID(body.ID)
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"staff": updated})
}

// ─── Phòng ────────────────────────────────────────────────────────────────

func (a *HKApp) handleRooms(w http.ResponseWriter, r *http.Request) {
	if _, err := a.hkAuthUser(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	rooms, err := a.store.ListRooms(false)
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không đọc được danh sách phòng.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"rooms": rooms})
}

func (a *HKApp) handleRoomsSync(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	if _, err := a.hkRequireAdmin(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	var body struct {
		Limit int `json:"limit"`
	}
	hkDecodeBody(r, &body)

	added, updated, err := a.hkSyncRooms(body.Limit)
	if err != nil {
		hkFail(w, http.StatusBadGateway, err.Error())
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{
		"added": added, "updated": updated, "synced_at": hkNowMs(),
	})
}

func (a *HKApp) handleRoomSettings(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	if _, err := a.hkRequireAdmin(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	var body struct {
		ID         string `json:"id"`
		TemplateID string `json:"template_id"`
		DoorNote   string `json:"door_note"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	if err := a.store.UpdateRoomSettings(body.ID, body.TemplateID, body.DoorNote); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được thay đổi.")
		return
	}
	room, _ := a.store.RoomByID(body.ID)
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"room": room})
}

// ─── Mẫu checklist ────────────────────────────────────────────────────────

func (a *HKApp) handleTemplates(w http.ResponseWriter, r *http.Request) {
	if _, err := a.hkAuthUser(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	if r.Method == http.MethodGet {
		templates, err := a.store.ListTemplates()
		if err != nil {
			hkFail(w, http.StatusInternalServerError, "Không đọc được mẫu checklist.")
			return
		}
		hkWriteJSON(w, http.StatusOK, map[string]interface{}{"templates": templates})
		return
	}

	if _, err := a.hkRequireAdmin(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	var t HKTemplate
	if err := hkDecodeBody(r, &t); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	if strings.TrimSpace(t.Name) == "" {
		hkFail(w, http.StatusBadRequest, "Mẫu chưa có tên.")
		return
	}
	// Mục không có tên là một ô trống trên điện thoại của cô, không biết phải làm
	// gì — chặn ngay lúc lưu thay vì để cô phát hiện lúc đứng trong phòng.
	for _, g := range t.Groups {
		for _, it := range g.Items {
			if strings.TrimSpace(it.Title) == "" {
				hkFail(w, http.StatusBadRequest, "Còn mục chưa có tên việc. Điền đủ rồi lưu lại nhé.")
				return
			}
		}
	}
	if t.ID == "" {
		t.ID = hkRandomID("hkt")
	}
	t.UpdatedAt = hkNowMs()
	if err := a.store.UpsertTemplate(t); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được mẫu.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"template": t})
}

// ─── Ca dọn ───────────────────────────────────────────────────────────────

func hkParseInt64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

func (a *HKApp) handleSessions(w http.ResponseWriter, r *http.Request) {
	u, err := a.hkAuthUser(r)
	if err != nil {
		hkFailAuth(w, err)
		return
	}
	q := r.URL.Query()
	f := HKSessionFilter{
		Day:     strings.TrimSpace(q.Get("day")),
		From:    hkParseInt64(q.Get("from")),
		To:      hkParseInt64(q.Get("to")),
		StaffID: strings.TrimSpace(q.Get("staff_id")),
		Status:  strings.TrimSpace(q.Get("status")),
	}
	// Cô dọn dẹp chỉ thấy ca của chính mình — ép ở backend, không tin tham số FE
	// gửi lên. Đây là dữ liệu lương của người khác.
	if u.Role != HKRoleAdmin {
		f.StaffID = u.ID
	}

	sessions, err := a.store.ListSessions(f)
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không đọc được danh sách ca.")
		return
	}
	views, err := a.hkBuildViews(sessions)
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không dựng được danh sách ca.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"sessions": views})
}

// hkLoadSessionFor lấy ca và kiểm quyền truy cập trong một bước — mọi handler
// thao tác trên ca đều phải đi qua đây, không có đường tắt.
func (a *HKApp) hkLoadSessionFor(w http.ResponseWriter, r *http.Request, id string) (HKUser, HKSession, bool) {
	u, err := a.hkAuthUser(r)
	if err != nil {
		hkFailAuth(w, err)
		return u, HKSession{}, false
	}
	sess, err := a.store.SessionByID(id)
	if err != nil {
		hkFail(w, http.StatusNotFound, "Không tìm thấy ca dọn này.")
		return u, sess, false
	}
	if u.Role != HKRoleAdmin && sess.StaffID != u.ID {
		hkFail(w, http.StatusForbidden, "Ca này không phải của bạn.")
		return u, sess, false
	}
	return u, sess, true
}

func (a *HKApp) writeSessionView(w http.ResponseWriter, sess HKSession) {
	views, err := a.hkBuildViews([]HKSession{sess})
	if err != nil || len(views) == 0 {
		hkFail(w, http.StatusInternalServerError, "Không dựng được dữ liệu ca.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"session": views[0]})
}

func (a *HKApp) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	_, sess, ok := a.hkLoadSessionFor(w, r, id)
	if !ok {
		return
	}
	a.writeSessionView(w, sess)
}

func (a *HKApp) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	_, sess, ok := a.hkLoadSessionFor(w, r, body.ID)
	if !ok {
		return
	}
	if sess.StartedAt == 0 {
		sess.StartedAt = hkNowMs()
	}
	if sess.Status == HKSessionTodo {
		sess.Status = HKSessionInProgress
	}
	if err := a.store.UpdateSession(sess); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được.")
		return
	}
	a.writeSessionView(w, sess)
}

// handleSessionItem ghi ảnh / tick cho một mục checklist.
//
// Tự chuyển sang "chờ đối soát" ngay khi đủ ảnh — đây là điều kiện đã chốt với
// nghiệp vụ: cô không phải bấm thêm nút nào nữa. Điều kiện đủ ảnh do BACKEND tự
// tính lại từ mẫu, không tin cờ FE gửi lên, vì nó quyết định có tiền hay không.
func (a *HKApp) handleSessionItem(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	var body struct {
		ID      string    `json:"id"`
		ItemID  string    `json:"item_id"`
		Photos  []HKPhoto `json:"photos"`
		Checked *bool     `json:"checked"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	_, sess, ok := a.hkLoadSessionFor(w, r, body.ID)
	if !ok {
		return
	}
	if sess.Status == HKSessionApproved {
		hkFail(w, http.StatusConflict, "Ca này quản lý đã duyệt xong, không sửa được nữa.")
		return
	}
	if strings.TrimSpace(body.ItemID) == "" {
		hkFail(w, http.StatusBadRequest, "Thiếu mã mục việc.")
		return
	}

	now := hkNowMs()
	if sess.ItemsState == nil {
		sess.ItemsState = map[string]HKItemState{}
	}
	st := sess.ItemsState[body.ItemID]
	if body.Photos != nil {
		clean := make([]HKPhoto, 0, len(body.Photos))
		for _, p := range body.Photos {
			// Chỉ nhận ảnh do chính server này cấp phát. Không có kiểm tra này thì
			// bất kỳ URL nào ngoài internet cũng thành "bằng chứng đã dọn".
			if !strings.HasPrefix(p.URL, "/api/hk/photo/") {
				continue
			}
			if p.UploadedAt == 0 {
				p.UploadedAt = now
			}
			clean = append(clean, p)
		}
		st.Photos = clean
		st.DoneAt = now
	}
	if body.Checked != nil {
		st.Checked = *body.Checked
		st.CheckedAt = now
	}
	sess.ItemsState[body.ItemID] = st

	if sess.StartedAt == 0 {
		sess.StartedAt = now
	}

	tpl := a.hkTemplateOf(&sess)
	progress := hkSessionProgress(&sess, tpl)
	switch {
	case progress.Complete && sess.Status != HKSessionRejected:
		if sess.SubmittedAt == 0 {
			sess.SubmittedAt = now
		}
		sess.Status = HKSessionSubmitted
	case sess.Status == HKSessionTodo:
		sess.Status = HKSessionInProgress
	}

	if err := a.store.UpdateSession(sess); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được ảnh. Thử lại nhé.")
		return
	}
	a.writeSessionView(w, sess)
}

func (a *HKApp) hkTemplateOf(sess *HKSession) *HKTemplate {
	var live *HKTemplate
	if sess.TemplateID != "" {
		if t, err := a.store.TemplateByID(sess.TemplateID); err == nil {
			live = &t
		}
	}
	return hkTemplateFor(sess, live)
}

func (a *HKApp) handleSessionSubmit(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	_, sess, ok := a.hkLoadSessionFor(w, r, body.ID)
	if !ok {
		return
	}
	progress := hkSessionProgress(&sess, a.hkTemplateOf(&sess))
	if !progress.Complete {
		msg := "Chưa đủ ảnh bắt buộc."
		if len(progress.Missing) > 0 {
			msg = "Còn thiếu ảnh ở: " + strings.Join(progress.Missing, ", ")
		}
		hkFail(w, http.StatusBadRequest, msg)
		return
	}
	sess.Status = HKSessionSubmitted
	if sess.SubmittedAt == 0 {
		sess.SubmittedAt = hkNowMs()
	}
	if err := a.store.UpdateSession(sess); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được.")
		return
	}
	a.writeSessionView(w, sess)
}

func (a *HKApp) handleSessionAssign(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	if _, err := a.hkRequireAdmin(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	var body struct {
		ID      string `json:"id"`
		StaffID string `json:"staff_id"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	sess, err := a.store.SessionByID(body.ID)
	if err != nil {
		hkFail(w, http.StatusNotFound, "Không tìm thấy ca dọn này.")
		return
	}
	if body.StaffID != "" {
		u, err := a.store.UserByID(body.StaffID)
		if err != nil || u.Role != HKRoleCleaner || u.Status != HKStaffActive {
			hkFail(w, http.StatusBadRequest, "Chỉ xếp được cho cô dọn dẹp đang làm việc.")
			return
		}
	}
	sess.StaffID = body.StaffID
	if err := a.store.UpdateSession(sess); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được.")
		return
	}
	a.writeSessionView(w, sess)
}

func (a *HKApp) handleSessionReview(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	admin, err := a.hkRequireAdmin(r)
	if err != nil {
		hkFailAuth(w, err)
		return
	}
	var body struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := hkDecodeBody(r, &body); err != nil {
		hkFail(w, http.StatusBadRequest, "Dữ liệu gửi lên không đọc được.")
		return
	}
	if body.Status != HKSessionApproved && body.Status != HKSessionRejected {
		hkFail(w, http.StatusBadRequest, "Chỉ duyệt hoặc từ chối được.")
		return
	}
	sess, err := a.store.SessionByID(body.ID)
	if err != nil {
		hkFail(w, http.StatusNotFound, "Không tìm thấy ca dọn này.")
		return
	}
	sess.Status = body.Status
	sess.ReviewNote = strings.TrimSpace(body.Note)
	sess.ReviewedAt = hkNowMs()
	sess.ReviewedBy = admin.Name
	if err := a.store.UpdateSession(sess); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được.")
		return
	}
	a.writeSessionView(w, sess)
}

// handleSessionsSync — quản lý bấm "Đồng bộ lịch": đọc iCal của mọi phòng và
// tạo ca cho các lượt khách sắp trả phòng.
func (a *HKApp) handleSessionsSync(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	if _, err := a.hkRequireAdmin(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	var body struct {
		Ahead int `json:"ahead"`
	}
	hkDecodeBody(r, &body)

	created, skipped, assigned, err := a.hkSyncSessions(body.Ahead)
	if err != nil {
		hkFail(w, http.StatusBadGateway, err.Error())
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{
		"created": created, "skipped": skipped, "assigned": assigned, "synced_at": hkNowMs(),
	})
}

// ─── Báo cáo hiệu suất ────────────────────────────────────────────────────
//
// Phần mềm này KHÔNG tính lương — lương tính theo cơ chế riêng ở ngoài. Ở đây chỉ
// đo việc đã làm: bao nhiêu ca, bao nhiêu phòng, dọn trung bình bao lâu.

func (a *HKApp) handleReport(w http.ResponseWriter, r *http.Request) {
	u, err := a.hkAuthUser(r)
	if err != nil {
		hkFailAuth(w, err)
		return
	}
	loc := time.Now().Location()
	q := r.URL.Query()

	// Mặc định xem theo NGÀY chứ không phải tháng: câu hỏi thường trực của quản lý
	// là "hôm nay chạy thế nào", không phải "tháng này tổng bao nhiêu".
	from, to, label, err := hkReportRange(q.Get("day"), q.Get("month"), loc)
	if err != nil {
		hkFail(w, http.StatusBadRequest, err.Error())
		return
	}

	f := HKSessionFilter{From: from.UnixMilli(), To: to.UnixMilli()}
	if u.Role != HKRoleAdmin {
		f.StaffID = u.ID
	}
	sessions, err := a.store.ListSessions(f)
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không đọc được dữ liệu ca.")
		return
	}
	for i := range sessions {
		sessions[i].Status = hkDeriveStatus(&sessions[i], a.hkTemplateOf(&sessions[i]))
	}

	users, err := a.store.ListUsers("")
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không đọc được danh sách nhân sự.")
		return
	}
	userByID := map[string]HKUser{}
	for _, x := range users {
		userByID[x.ID] = x
	}

	progressOf := func(s *HKSession) HKProgress {
		return hkSessionProgress(s, a.hkTemplateOf(s))
	}
	rows := hkBuildPerf(sessions, userByID, progressOf)

	total := HKPerfRow{Name: "Tổng"}
	roomSet := map[string]bool{}
	minuteSum, minuteN := 0, 0
	for i := range sessions {
		s := &sessions[i]
		if m := hkSessionMinutes(s); m > 0 && (s.Status == HKSessionApproved || s.Status == HKSessionSubmitted) {
			minuteSum += m
			minuteN++
		}
	}
	for _, x := range rows {
		total.Sessions += x.Sessions
		total.Approved += x.Approved
		total.Pending += x.Pending
		total.Rejected += x.Rejected
		total.Late += x.Late
		total.Photos += x.Photos
	}
	for i := range sessions {
		if sessions[i].Status == HKSessionApproved || sessions[i].Status == HKSessionSubmitted {
			roomSet[sessions[i].RoomID] = true
		}
	}
	total.Rooms = len(roomSet)
	if minuteN > 0 {
		total.AvgMinute = minuteSum / minuteN
	}

	views, err := a.hkBuildViews(sessions)
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không dựng được báo cáo.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{
		"label": label, "rows": rows, "total": total, "sessions": views,
	})
}

// hkReportRange nhận `day` (YYYY-MM-DD) hoặc `month` (YYYY-MM); không có thì lấy
// hôm nay.
func hkReportRange(day, month string, loc *time.Location) (time.Time, time.Time, string, error) {
	day, month = strings.TrimSpace(day), strings.TrimSpace(month)
	if month != "" {
		start, err := time.ParseInLocation("2006-01", month, loc)
		if err != nil {
			return time.Time{}, time.Time{}, "", fmt.Errorf("Tháng không hợp lệ. Dạng đúng: 2026-08")
		}
		return start, start.AddDate(0, 1, 0).Add(-time.Millisecond), month, nil
	}
	if day == "" {
		day = time.Now().In(loc).Format("2006-01-02")
	}
	start, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil {
		return time.Time{}, time.Time{}, "", fmt.Errorf("Ngày không hợp lệ. Dạng đúng: 2026-08-14")
	}
	return start, start.AddDate(0, 0, 1).Add(-time.Millisecond), day, nil
}

// ─── Ảnh ──────────────────────────────────────────────────────────────────

var hkPhotoExts = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/heic": ".heic",
}

// handlePhotoUpload nhận ảnh checklist.
//
// Ảnh là CHỨNG TỪ CHẤM CÔNG nên lưu xuống đĩa cạnh database, không phải thư mục
// tạm như phần render video của repo này (thư mục tạm bị hệ điều hành dọn).
func (a *HKApp) handlePhotoUpload(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	u, err := a.hkAuthUser(r)
	if err != nil {
		hkFailAuth(w, err)
		return
	}
	// 25MB: ảnh điện thoại đời mới ~5-8MB, chừa chỗ cho ảnh chưa nén.
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		hkFail(w, http.StatusBadRequest, "Ảnh quá lớn hoặc gửi lên bị lỗi.")
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		hkFail(w, http.StatusBadRequest, "Không nhận được ảnh.")
		return
	}
	defer file.Close()

	// Nhận diện kiểu file bằng nội dung thật, không tin đuôi tên hay Content-Type
	// do trình duyệt khai — đây là dữ liệu người ngoài gửi lên.
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	head = head[:n]
	ctype := http.DetectContentType(head)
	ext, ok := hkPhotoExts[ctype]
	if !ok {
		hkFail(w, http.StatusBadRequest, "Chỉ nhận ảnh JPG, PNG hoặc WEBP.")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không đọc lại được ảnh.")
		return
	}

	name := hkRandomID("p") + ext
	dst, err := os.Create(filepath.Join(a.photoDir, name))
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được ảnh.")
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		hkFail(w, http.StatusInternalServerError, "Không lưu được ảnh.")
		return
	}

	_ = header
	_ = u
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{
		"url":         "/api/hk/photo/" + name,
		"uploaded_at": hkNowMs(),
	})
}

// handlePhotoServe phục vụ ảnh đã lưu. CÓ kiểm đăng nhập: ảnh chụp bên trong nhà
// khách, để công khai là rò rỉ chỗ ở của người khác.
func (a *HKApp) handlePhotoServe(w http.ResponseWriter, r *http.Request) {
	if _, err := a.hkAuthUser(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/hk/photo/")
	// Chặn leo thư mục: chỉ nhận đúng một tên file, không nhận đường dẫn.
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		hkFail(w, http.StatusBadRequest, "Tên ảnh không hợp lệ.")
		return
	}
	path := filepath.Join(a.photoDir, name)
	if _, err := os.Stat(path); err != nil {
		hkFail(w, http.StatusNotFound, "Không tìm thấy ảnh.")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, path)
}

// ─── Đăng ký route ────────────────────────────────────────────────────────

func (a *HKApp) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/hk/login", a.handleLogin)
	mux.HandleFunc("/api/hk/register", a.handleRegister)
	mux.HandleFunc("/api/hk/logout", a.handleLogout)
	mux.HandleFunc("/api/hk/me", a.handleMe)
	mux.HandleFunc("/api/hk/meta", a.handleMeta)

	mux.HandleFunc("/api/hk/staffs", a.handleStaffs)
	mux.HandleFunc("/api/hk/staffs/review", a.handleStaffReview)

	mux.HandleFunc("/api/hk/rooms", a.handleRooms)
	mux.HandleFunc("/api/hk/rooms/sync", a.handleRoomsSync)
	mux.HandleFunc("/api/hk/rooms/settings", a.handleRoomSettings)

	mux.HandleFunc("/api/hk/templates", a.handleTemplates)

	mux.HandleFunc("/api/hk/sessions", a.handleSessions)
	mux.HandleFunc("/api/hk/sessions/get", a.handleSessionGet)
	mux.HandleFunc("/api/hk/sessions/sync", a.handleSessionsSync)
	mux.HandleFunc("/api/hk/sessions/start", a.handleSessionStart)
	mux.HandleFunc("/api/hk/sessions/item", a.handleSessionItem)
	mux.HandleFunc("/api/hk/sessions/submit", a.handleSessionSubmit)
	mux.HandleFunc("/api/hk/sessions/assign", a.handleSessionAssign)
	mux.HandleFunc("/api/hk/sessions/review", a.handleSessionReview)

	mux.HandleFunc("/api/hk/report", a.handleReport)
	mux.HandleFunc("/api/hk/reviews", a.handleReviews)
	mux.HandleFunc("/api/hk/reviews/sync", a.handleReviewsSync)

	mux.HandleFunc("/api/hk/photos", a.handlePhotoUpload)
	mux.HandleFunc("/api/hk/photo/", a.handlePhotoServe)
}
