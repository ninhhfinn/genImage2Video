package main

// Test tích hợp đi qua ĐÚNG đường của bản thật: HTTP → handler → SQLite.
//
// Đây là chỗ bắt những lỗi mà unit test hàm thuần không thấy: quên kiểm quyền,
// cô này đọc được ca của cô kia, ảnh giả lọt vào làm bằng chứng, ca đủ ảnh mà
// không tự ghi công.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type hkTestEnv struct {
	t          *testing.T
	app        *HKApp
	mux        *http.ServeMux
	adminToken string
	lanToken   string
	hoaToken   string
	lanID      string
	hoaID      string
	roomID     string
}

func newHKTestEnv(t *testing.T) *hkTestEnv {
	t.Helper()
	t.Setenv("HK_ADMIN_PHONE", "0900000000")
	t.Setenv("HK_ADMIN_PASSWORD", "admin-secret")

	app, err := NewHKApp(t.TempDir())
	if err != nil {
		t.Fatalf("khởi tạo app: %v", err)
	}
	t.Cleanup(func() { app.store.Close() })

	mux := http.NewServeMux()
	app.Register(mux)
	env := &hkTestEnv{t: t, app: app, mux: mux}

	env.adminToken = env.login("0900000000", "admin-secret")

	// Một phòng thật-hình-dạng, không gọi mạng: test không được phụ thuộc
	// api.dayladau.com đang sống hay không.
	room := HKRoom{
		ID: "ls_test_01", ListingID: "ls_test_01", Code: "CG-TEST01",
		Name: "Căn thử nghiệm", Address: "1 Cầu Giấy, Hà Nội", Zone: "Cầu Giấy",
		RoomType: "one_bedroom", HostName: "Chủ nhà thử", TemplateID: "hkt_studio",
		CleanTime: 1, CheckinHr: 14, CheckoutHr: 11, Active: true,
	}
	if err := app.store.UpsertRoom(room); err != nil {
		t.Fatalf("tạo phòng: %v", err)
	}
	env.roomID = room.ID

	env.lanID = env.registerAndApprove("Nguyễn Thị Lan", "0912345601", "123456", "Cầu Giấy")
	env.hoaID = env.registerAndApprove("Trần Thị Hoa", "0912345602", "123456", "Hoàn Kiếm")
	env.lanToken = env.login("0912345601", "123456")
	env.hoaToken = env.login("0912345602", "123456")
	return env
}

// ─── Tiện ích gọi HTTP ────────────────────────────────────────────────────

func (e *hkTestEnv) do(method, path, token string, body interface{}) *httptest.ResponseRecorder {
	e.t.Helper()
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		r.Header.Set("X-HK-Token", token)
	}
	w := httptest.NewRecorder()
	e.mux.ServeHTTP(w, r)
	return w
}

func (e *hkTestEnv) decode(w *httptest.ResponseRecorder, v interface{}) {
	e.t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		e.t.Fatalf("không đọc được phản hồi (%d): %s", w.Code, w.Body.String())
	}
}

func (e *hkTestEnv) login(phone, password string) string {
	e.t.Helper()
	w := e.do("POST", "/api/hk/login", "", map[string]string{"phone": phone, "password": password})
	if w.Code != 200 {
		e.t.Fatalf("đăng nhập %s hỏng (%d): %s", phone, w.Code, w.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	e.decode(w, &out)
	return out.Token
}

func (e *hkTestEnv) registerAndApprove(name, phone, password, zone string) string {
	e.t.Helper()
	w := e.do("POST", "/api/hk/register", "", map[string]string{
		"name": name, "phone": phone, "password": password, "zone": zone,
	})
	if w.Code != 200 {
		e.t.Fatalf("đăng ký hỏng (%d): %s", w.Code, w.Body.String())
	}
	var out struct {
		User HKUser `json:"user"`
	}
	e.decode(w, &out)
	if w2 := e.do("POST", "/api/hk/staffs/review", e.adminToken, map[string]string{
		"id": out.User.ID, "status": HKStaffActive,
	}); w2.Code != 200 {
		e.t.Fatalf("duyệt tài khoản hỏng (%d): %s", w2.Code, w2.Body.String())
	}
	return out.User.ID
}

// createSession tạo ca qua đúng đường mà đồng bộ iCal dùng, chỉ khác là tự đặt
// mã lượt đặt để test không phụ thuộc api.dayladau.com đang sống hay không.
func (e *hkTestEnv) createSession(day string) HKSessionView {
	e.t.Helper()
	return e.createSessionUID(day, "uid-"+day+"-"+e.roomID)
}

func (e *hkTestEnv) createSessionUID(day, uid string) HKSessionView {
	e.t.Helper()
	room, err := e.app.store.RoomByID(e.roomID)
	if err != nil {
		e.t.Fatalf("đọc phòng: %v", err)
	}
	loc := time.Now().Location()
	d, _ := time.ParseInLocation("2006-01-02", day, loc)
	turn := hkTurn{
		RoomID: room.ID, ListingID: room.ListingID, UID: uid, Day: day,
		CheckoutAt: d.Add(11 * time.Hour), DeadlineAt: d.Add(14 * time.Hour),
	}
	if _, err := e.app.hkCreateSessionFromTurn(room, turn); err != nil {
		e.t.Fatalf("tạo ca: %v", err)
	}
	sess, err := e.app.store.SessionByID("hks_" + hkShortHash(uid))
	if err != nil {
		e.t.Fatalf("đọc ca vừa tạo: %v", err)
	}
	views, err := e.app.hkBuildViews([]HKSession{sess})
	if err != nil || len(views) == 0 {
		e.t.Fatalf("dựng view: %v", err)
	}
	return views[0]
}

// uploadPhoto đẩy một ảnh PNG thật (1×1) qua đúng endpoint upload.
func (e *hkTestEnv) uploadPhoto(token string) string {
	e.t.Helper()
	png := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
		0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("photo", "anh.png")
	fw.Write(png)
	mw.Close()

	r := httptest.NewRequest("POST", "/api/hk/photos", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("X-HK-Token", token)
	w := httptest.NewRecorder()
	e.mux.ServeHTTP(w, r)
	if w.Code != 200 {
		e.t.Fatalf("upload ảnh hỏng (%d): %s", w.Code, w.Body.String())
	}
	var out struct {
		URL string `json:"url"`
	}
	e.decode(w, &out)
	return out.URL
}

// fillAllRequired chụp đủ ảnh cho mọi mục bắt buộc của ca.
func (e *hkTestEnv) fillAllRequired(sessionID, token string) HKSessionView {
	e.t.Helper()
	sess, err := e.app.store.SessionByID(sessionID)
	if err != nil {
		e.t.Fatalf("đọc ca: %v", err)
	}
	tpl := e.app.hkTemplateOf(&sess)
	var last HKSessionView
	for _, it := range hkFlattenItems(tpl) {
		if !it.RequirePhoto {
			continue
		}
		photos := []map[string]interface{}{}
		for i := 0; i < hkMinPhotos(it); i++ {
			photos = append(photos, map[string]interface{}{"url": e.uploadPhoto(token)})
		}
		w := e.do("POST", "/api/hk/sessions/item", token, map[string]interface{}{
			"id": sessionID, "item_id": it.ID, "photos": photos,
		})
		if w.Code != 200 {
			e.t.Fatalf("ghi ảnh mục %s hỏng (%d): %s", it.ID, w.Code, w.Body.String())
		}
		var out struct {
			Session HKSessionView `json:"session"`
		}
		e.decode(w, &out)
		last = out.Session
	}
	return last
}

func today() string { return time.Now().Format("2006-01-02") }

// ─── Đăng nhập & duyệt tài khoản ──────────────────────────────────────────

func TestRegisterRequiresApproval(t *testing.T) {
	e := newHKTestEnv(t)
	w := e.do("POST", "/api/hk/register", "", map[string]string{
		"name": "Cô Mới", "phone": "0912345699", "password": "123456", "zone": "Tây Hồ",
	})
	if w.Code != 200 {
		t.Fatalf("đăng ký phải thành công: %s", w.Body.String())
	}
	// Chưa duyệt thì không đăng nhập được, và phải nói rõ lý do cho cô đọc.
	w = e.do("POST", "/api/hk/login", "", map[string]string{"phone": "0912345699", "password": "123456"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("muốn 403 được %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "chờ quản lý duyệt") {
		t.Fatalf("thông báo phải nói rõ đang chờ duyệt: %s", w.Body.String())
	}
}

func TestLoginDoesNotRevealWhoIsRegistered(t *testing.T) {
	e := newHKTestEnv(t)
	unknown := e.do("POST", "/api/hk/login", "", map[string]string{"phone": "0999999999", "password": "x"})
	wrongPw := e.do("POST", "/api/hk/login", "", map[string]string{"phone": "0912345601", "password": "sai"})
	if unknown.Body.String() != wrongPw.Body.String() {
		t.Fatalf("số chưa đăng ký và sai mật khẩu phải trả cùng một câu:\n%s\n%s",
			unknown.Body.String(), wrongPw.Body.String())
	}
}

func TestDuplicatePhoneRejected(t *testing.T) {
	e := newHKTestEnv(t)
	w := e.do("POST", "/api/hk/register", "", map[string]string{
		"name": "Trùng", "phone": "0912345601", "password": "123456",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("muốn 409 được %d: %s", w.Code, w.Body.String())
	}
}

// Số điện thoại gõ kiểu nào cũng phải ra một tài khoản.
func TestPhoneNormalizationOnLogin(t *testing.T) {
	e := newHKTestEnv(t)
	for _, phone := range []string{"0912345601", "0912 345 601", "+84912345601"} {
		if tok := e.login(phone, "123456"); tok == "" {
			t.Fatalf("đăng nhập bằng %q phải được", phone)
		}
	}
}

func TestSuspendedTokenStopsWorking(t *testing.T) {
	e := newHKTestEnv(t)
	if w := e.do("GET", "/api/hk/me", e.lanToken, nil); w.Code != 200 {
		t.Fatalf("trước khi khoá phải vào được: %d", w.Code)
	}
	e.do("POST", "/api/hk/staffs/review", e.adminToken, map[string]string{
		"id": e.lanID, "status": HKStaffSuspended,
	})
	// Token cũ phải mất tác dụng NGAY, không đợi hết hạn 30 ngày.
	if w := e.do("GET", "/api/hk/me", e.lanToken, nil); w.Code != http.StatusForbidden {
		t.Fatalf("sau khi khoá muốn 403 được %d", w.Code)
	}
}

// ─── Phân quyền ───────────────────────────────────────────────────────────

func TestCleanerCannotUseAdminEndpoints(t *testing.T) {
	e := newHKTestEnv(t)
	sess := e.createSession(today())
	cases := []struct {
		path string
		body interface{}
	}{
		{"/api/hk/staffs/review", map[string]string{"id": e.hoaID, "status": HKStaffActive}},
		{"/api/hk/sessions/assign", map[string]string{"id": sess.ID, "staff_id": e.hoaID}},
		{"/api/hk/sessions/review", map[string]string{"id": sess.ID, "status": HKSessionApproved}},
		{"/api/hk/rooms/sync", map[string]int{"limit": 1}},
		{"/api/hk/rooms/settings", map[string]interface{}{"id": e.roomID, "base_fee": 1}},
	}
	for _, c := range cases {
		if w := e.do("POST", c.path, e.lanToken, c.body); w.Code != http.StatusForbidden {
			t.Errorf("%s: cô dọn dẹp phải bị chặn, được %d: %s", c.path, w.Code, w.Body.String())
		}
	}
	// GET danh sách nhân sự cũng là dữ liệu cá nhân của người khác.
	if w := e.do("GET", "/api/hk/staffs", e.lanToken, nil); w.Code != http.StatusForbidden {
		t.Errorf("GET /staffs: muốn 403 được %d", w.Code)
	}
}

func TestCleanerCannotTouchAnotherCleanersSession(t *testing.T) {
	e := newHKTestEnv(t)
	sess := e.createSession(today())
	e.do("POST", "/api/hk/sessions/assign", e.adminToken, map[string]string{
		"id": sess.ID, "staff_id": e.lanID,
	})
	// Hoa không được xem, cũng không được ghi ảnh vào ca của Lan.
	if w := e.do("GET", "/api/hk/sessions/get?id="+sess.ID, e.hoaToken, nil); w.Code != http.StatusForbidden {
		t.Errorf("xem ca người khác: muốn 403 được %d", w.Code)
	}
	w := e.do("POST", "/api/hk/sessions/item", e.hoaToken, map[string]interface{}{
		"id": sess.ID, "item_id": "i_bed_linen", "photos": []interface{}{},
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("ghi vào ca người khác: muốn 403 được %d", w.Code)
	}
}

// Cô gửi staff_id của người khác lên cũng chỉ nhận về ca của chính mình.
func TestCleanerListIsForcedToOwnSessions(t *testing.T) {
	e := newHKTestEnv(t)
	sess := e.createSession(today())
	e.do("POST", "/api/hk/sessions/assign", e.adminToken, map[string]string{
		"id": sess.ID, "staff_id": e.lanID,
	})
	w := e.do("GET", "/api/hk/sessions?staff_id="+e.lanID, e.hoaToken, nil)
	if w.Code != 200 {
		t.Fatalf("muốn 200 được %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Sessions []HKSessionView `json:"sessions"`
	}
	e.decode(w, &out)
	for _, s := range out.Sessions {
		if s.StaffID != e.hoaID {
			t.Fatalf("Hoa nhận được ca của %s — rò rỉ dữ liệu lương người khác", s.StaffID)
		}
	}
}

func TestNoTokenRejected(t *testing.T) {
	e := newHKTestEnv(t)
	for _, p := range []string{"/api/hk/sessions", "/api/hk/rooms", "/api/hk/me", "/api/hk/report"} {
		if w := e.do("GET", p, "", nil); w.Code != http.StatusUnauthorized {
			t.Errorf("%s không token: muốn 401 được %d", p, w.Code)
		}
	}
}

// ─── Vòng đời ca dọn ──────────────────────────────────────────────────────

func TestFullLifecycleAutoRecordsPay(t *testing.T) {
	e := newHKTestEnv(t)
	sess := e.createSession(today())
	e.do("POST", "/api/hk/sessions/assign", e.adminToken, map[string]string{
		"id": sess.ID, "staff_id": e.lanID,
	})

	final := e.fillAllRequired(sess.ID, e.lanToken)

	// Đủ ảnh là TỰ chuyển chờ đối soát và ghi công — cô không phải bấm nút nào nữa.
	if final.Status != HKSessionSubmitted {
		t.Fatalf("muốn submitted được %s (tiến độ %+v)", final.Status, final.Progress)
	}
	if !final.Progress.Complete {
		t.Fatalf("phải đủ ảnh: %+v", final.Progress)
	}

	// Quản lý duyệt kèm trừ tiền.
	w := e.do("POST", "/api/hk/sessions/review", e.adminToken, map[string]interface{}{
		"id": sess.ID, "status": HKSessionApproved, "note": "Thiếu giấy vệ sinh",
	})
	if w.Code != 200 {
		t.Fatalf("duyệt hỏng: %s", w.Body.String())
	}
	var out struct {
		Session HKSessionView `json:"session"`
	}
	e.decode(w, &out)
	if out.Session.Status != HKSessionApproved {
		t.Fatalf("muốn approved được %s", out.Session.Status)
	}
	if out.Session.ReviewNote != "Thiếu giấy vệ sinh" {
		t.Fatalf("ghi chú hậu kiểm phải lưu để cô đọc được: %q", out.Session.ReviewNote)
	}
}

// Chỉ nhận ảnh do chính server này cấp phát — nếu không, dán URL bất kỳ ngoài
// internet là thành "bằng chứng đã dọn".
func TestForeignPhotoURLRejected(t *testing.T) {
	e := newHKTestEnv(t)
	sess := e.createSession(today())
	e.do("POST", "/api/hk/sessions/assign", e.adminToken, map[string]string{
		"id": sess.ID, "staff_id": e.lanID,
	})
	w := e.do("POST", "/api/hk/sessions/item", e.lanToken, map[string]interface{}{
		"id": sess.ID, "item_id": "i_bed_linen",
		"photos": []map[string]string{{"url": "https://example.com/anh-gia.jpg"}},
	})
	if w.Code != 200 {
		t.Fatalf("request hợp lệ, chỉ ảnh bị loại: %s", w.Body.String())
	}
	var out struct {
		Session HKSessionView `json:"session"`
	}
	e.decode(w, &out)
	if len(out.Session.ItemsState["i_bed_linen"].Photos) != 0 {
		t.Fatal("ảnh từ tên miền ngoài không được nhận làm bằng chứng")
	}
}

func TestSubmitBlockedWhenPhotosMissing(t *testing.T) {
	e := newHKTestEnv(t)
	sess := e.createSession(today())
	e.do("POST", "/api/hk/sessions/assign", e.adminToken, map[string]string{
		"id": sess.ID, "staff_id": e.lanID,
	})
	w := e.do("POST", "/api/hk/sessions/submit", e.lanToken, map[string]string{"id": sess.ID})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("muốn 400 được %d", w.Code)
	}
	// Thông báo phải nêu TÊN mục còn thiếu — cô cần biết quay lại phòng nào.
	if !strings.Contains(w.Body.String(), "Còn thiếu ảnh ở:") {
		t.Fatalf("phải nêu mục còn thiếu: %s", w.Body.String())
	}
}

func TestApprovedSessionIsLocked(t *testing.T) {
	e := newHKTestEnv(t)
	sess := e.createSession(today())
	e.do("POST", "/api/hk/sessions/assign", e.adminToken, map[string]string{
		"id": sess.ID, "staff_id": e.lanID,
	})
	e.fillAllRequired(sess.ID, e.lanToken)
	e.do("POST", "/api/hk/sessions/review", e.adminToken, map[string]interface{}{
		"id": sess.ID, "status": HKSessionApproved,
	})
	w := e.do("POST", "/api/hk/sessions/item", e.lanToken, map[string]interface{}{
		"id": sess.ID, "item_id": "i_bed_linen", "photos": []interface{}{},
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("ca đã chốt công không được sửa nữa, muốn 409 được %d", w.Code)
	}
}
func TestReportScopedByRole(t *testing.T) {
	e := newHKTestEnv(t)
	sess := e.createSession(today())
	e.do("POST", "/api/hk/sessions/assign", e.adminToken, map[string]string{
		"id": sess.ID, "staff_id": e.lanID,
	})
	e.fillAllRequired(sess.ID, e.lanToken)

	day := today()

	var adminOut struct {
		Rows  []HKPerfRow `json:"rows"`
		Total HKPerfRow   `json:"total"`
	}
	e.decode(e.do("GET", "/api/hk/report?day="+day, e.adminToken, nil), &adminOut)
	if len(adminOut.Rows) != 1 || adminOut.Rows[0].Sessions != 1 {
		t.Fatalf("quản lý phải thấy báo cáo: %+v", adminOut.Rows)
	}
	if adminOut.Total.Rooms != 1 {
		t.Fatalf("tổng số phòng sai: %+v", adminOut.Total)
	}

	// Hoa chưa làm ca nào → báo cáo của Hoa rỗng, không thấy số liệu của Lan.
	var hoaOut struct {
		Rows []HKPerfRow `json:"rows"`
	}
	e.decode(e.do("GET", "/api/hk/report?day="+day, e.hoaToken, nil), &hoaOut)
	if len(hoaOut.Rows) != 0 {
		t.Fatalf("Hoa không được thấy số liệu người khác: %+v", hoaOut.Rows)
	}
}
func TestPhotoRequiresLogin(t *testing.T) {
	e := newHKTestEnv(t)
	url := e.uploadPhoto(e.lanToken)
	// Ảnh chụp bên trong nhà khách — không được để công khai.
	if w := e.do("GET", url, "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("xem ảnh không đăng nhập: muốn 401 được %d", w.Code)
	}
	if w := e.do("GET", url, e.lanToken, nil); w.Code != 200 {
		t.Fatalf("đã đăng nhập phải xem được: %d", w.Code)
	}
}

func TestPhotoRejectsNonImage(t *testing.T) {
	e := newHKTestEnv(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("photo", "script.png") // đuôi ảnh, ruột không phải ảnh
	fw.Write([]byte("#!/bin/sh\necho hacked\n"))
	mw.Close()

	r := httptest.NewRequest("POST", "/api/hk/photos", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("X-HK-Token", e.lanToken)
	w := httptest.NewRecorder()
	e.mux.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("đuôi .png nhưng ruột không phải ảnh phải bị chặn, được %d", w.Code)
	}
}

func TestPhotoPathTraversalBlocked(t *testing.T) {
	e := newHKTestEnv(t)
	for _, name := range []string{"../housekeeping.db", "..%2Fhousekeeping.db", "a/b.png"} {
		w := e.do("GET", "/api/hk/photo/"+name, e.lanToken, nil)
		if w.Code == 200 {
			t.Errorf("%q không được trả file: %d", name, w.Code)
		}
	}
}

// ─── Mẫu checklist ────────────────────────────────────────────────────────

func TestTemplateRejectsEmptyItemTitle(t *testing.T) {
	e := newHKTestEnv(t)
	w := e.do("POST", "/api/hk/templates", e.adminToken, HKTemplate{
		ID: "hkt_studio", Name: "Thử", Groups: []HKGroup{
			{ID: "g", Title: "Nhóm", Items: []HKItem{{ID: "i", Title: "  ", RequirePhoto: true}}},
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("mục trống tên phải bị chặn lúc lưu, được %d", w.Code)
	}
}

// Sửa mẫu KHÔNG được đổi điều kiện của ca đang dở — cô đang dọn không thể bỗng
// thiếu ảnh cho một mục mà lúc bắt đầu chưa hề tồn tại.
func TestEditingTemplateDoesNotBreakOpenSession(t *testing.T) {
	e := newHKTestEnv(t)
	sess := e.createSession(today())
	e.do("POST", "/api/hk/sessions/assign", e.adminToken, map[string]string{
		"id": sess.ID, "staff_id": e.lanID,
	})
	e.fillAllRequired(sess.ID, e.lanToken)

	// Thêm một mục bắt buộc mới vào mẫu đang dùng.
	tpl, _ := e.app.store.TemplateByID("hkt_studio")
	tpl.Groups = append(tpl.Groups, HKGroup{
		ID: "g_moi", Title: "Nhóm mới", Items: []HKItem{
			{ID: "i_moi", Title: "Việc mới thêm", RequirePhoto: true},
		},
	})
	if w := e.do("POST", "/api/hk/templates", e.adminToken, tpl); w.Code != 200 {
		t.Fatalf("lưu mẫu hỏng: %s", w.Body.String())
	}

	var out struct {
		Session HKSessionView `json:"session"`
	}
	e.decode(e.do("GET", "/api/hk/sessions/get?id="+sess.ID, e.adminToken, nil), &out)
	if out.Session.Status != HKSessionSubmitted {
		t.Fatalf("ca đã nộp phải giữ nguyên trạng thái, được %s (tiến độ %+v)",
			out.Session.Status, out.Session.Progress)
	}
}

// ─── Lưu trữ ──────────────────────────────────────────────────────────────

func TestDataSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HK_ADMIN_PHONE", "0900000000")
	t.Setenv("HK_ADMIN_PASSWORD", "admin-secret")

	app, err := NewHKApp(dir)
	if err != nil {
		t.Fatalf("mở lần 1: %v", err)
	}
	app.store.UpsertRoom(HKRoom{ID: "r1", Name: "Phòng A", Active: true})
	app.store.Close()

	// Khởi động lại: dữ liệu phải còn, và KHÔNG được tạo thêm admin thứ hai.
	app2, err := NewHKApp(dir)
	if err != nil {
		t.Fatalf("mở lần 2: %v", err)
	}
	defer app2.store.Close()

	room, err := app2.store.RoomByID("r1")
	if err != nil || room.Name != "Phòng A" {
		t.Fatalf("phòng phải còn sau khi khởi động lại: %+v %v", room, err)
	}
	admins, _ := app2.store.ListUsers(HKRoleAdmin)
	if len(admins) != 1 {
		t.Fatalf("chỉ được có 1 tài khoản quản lý, có %d", len(admins))
	}
	if _, err := app2.store.TemplateByID("hkt_studio"); err != nil {
		t.Fatalf("mẫu checklist phải còn: %v", err)
	}
}

// Đồng bộ lại từ Dayladau không được xoá công sửa tay của quản lý.
func TestRoomSyncKeepsManualSettings(t *testing.T) {
	e := newHKTestEnv(t)
	e.do("POST", "/api/hk/rooms/settings", e.adminToken, map[string]interface{}{
		"id": e.roomID, "template_id": "hkt_2pn", "door_note": "Mã cổng 8899",
	})
	// Giả lập một lượt đồng bộ ghi đè các trường lấy từ API.
	e.app.store.UpsertRoom(HKRoom{
		ID: e.roomID, ListingID: e.roomID, Name: "Tên mới từ Dayladau",
		Zone: "Cầu Giấy", RoomType: "one_bedroom", Active: true, SyncedAt: hkNowMs(),
		TemplateID: "hkt_studio", DoorNote: "",
	})
	room, _ := e.app.store.RoomByID(e.roomID)
	if room.Name != "Tên mới từ Dayladau" {
		t.Fatalf("trường từ API phải được cập nhật: %q", room.Name)
	}
	if room.TemplateID != "hkt_2pn" || room.DoorNote != "Mã cổng 8899" {
		t.Fatalf("cài đặt tay bị đồng bộ ghi đè mất: %+v", room)
	}
}

func TestStoreRejectsUnknownTableInCount(t *testing.T) {
	e := newHKTestEnv(t)
	if _, err := e.app.store.CountRows("hk_user; DROP TABLE hk_user"); err == nil {
		t.Fatal("tên bảng lạ phải bị từ chối")
	}
}

func TestPhotoStoredOnDisk(t *testing.T) {
	e := newHKTestEnv(t)
	url := e.uploadPhoto(e.lanToken)
	name := strings.TrimPrefix(url, "/api/hk/photo/")
	if _, err := filepath.Abs(filepath.Join(e.app.photoDir, name)); err != nil {
		t.Fatalf("đường dẫn ảnh hỏng: %v", err)
	}
	if w := e.do("GET", url, e.adminToken, nil); w.Code != 200 {
		t.Fatalf("ảnh phải đọc lại được từ đĩa: %d", w.Code)
	}
}

// Gợi ý người phụ trách phải chạy lại cho ca CŨ còn trống.
//
// Tình huống thật gặp lúc test: ca sinh ra trước khi có cô nào nhận khu đó, nên
// nằm mãi ở "chưa xếp"; bấm Đồng bộ lịch lần nữa cũng không sửa vì ca đã tồn tại.
// Quản lý nhìn danh sách trống người mà không hiểu vì sao.
func TestSyncSuggestsStaffForExistingUnassignedSessions(t *testing.T) {
	e := newHKTestEnv(t)
	day := today()

	// Ca tạo khi chưa có ai nhận khu "Cầu Giấy" của phòng test.
	sess := e.createSessionUID(day, "uid-chua-xep")
	if err := e.app.store.AssignSessionStaff(sess.ID, ""); err != nil {
		t.Fatal(err)
	}

	// Giờ mới có cô nhận đúng khu đó.
	e.do("POST", "/api/hk/staffs/review", e.adminToken, map[string]string{
		"id": e.lanID, "status": HKStaffActive,
	})

	n, err := e.app.hkSuggestStaffForOpenSessions(day, day)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("muốn gán được 1 ca, được %d", n)
	}
	after, _ := e.app.store.SessionByID(sess.ID)
	if after.StaffID != e.lanID {
		t.Fatalf("ca phải được gán cho cô nhận khu Cầu Giấy, được %q", after.StaffID)
	}
}

// Ca đang dở KHÔNG được đổi người — cướp việc giữa chừng.
func TestSyncDoesNotReassignStartedSession(t *testing.T) {
	e := newHKTestEnv(t)
	day := today()
	sess := e.createSessionUID(day, "uid-dang-do")
	e.app.store.AssignSessionStaff(sess.ID, "")

	// Cô Hoa đã bấm bắt đầu (dù chưa được xếp chính thức).
	s, _ := e.app.store.SessionByID(sess.ID)
	s.StartedAt = hkNowMs()
	if err := e.app.store.UpdateSession(s); err != nil {
		t.Fatal(err)
	}

	n, err := e.app.hkSuggestStaffForOpenSessions(day, day)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("ca đã bắt đầu không được gán lại, gán %d ca", n)
	}
}

// Không có ai nhận khu đó thì để trống cho quản lý xếp tay, không gán bừa.
func TestSuggestLeavesBlankWhenNoZoneMatch(t *testing.T) {
	e := newHKTestEnv(t)
	day := today()
	sess := e.createSessionUID(day, "uid-khong-khop")
	e.app.store.AssignSessionStaff(sess.ID, "")

	// Đổi khu của phòng sang nơi không cô nào nhận.
	room, _ := e.app.store.RoomByID(e.roomID)
	room.Zone = "Cà Mau"
	e.app.store.UpsertRoom(room)

	n, _ := e.app.hkSuggestStaffForOpenSessions(day, day)
	if n != 0 {
		t.Fatalf("không ai nhận khu đó thì phải để trống, gán %d", n)
	}
	after, _ := e.app.store.SessionByID(sess.ID)
	if after.StaffID != "" {
		t.Fatalf("không được gán bừa cho người khu khác: %q", after.StaffID)
	}
}

// ─── Lọc đánh giá ─────────────────────────────────────────────────────────

func (e *hkTestEnv) seedReviews(t *testing.T) {
	t.Helper()
	base := hkNowMs()
	day := int64(86400000)
	list := []HKReview{
		{ID: "r1", RoomID: e.roomID, RoomCode: "CG-TEST01", ListingName: "Căn thử nghiệm",
			FacilityID: 309, FacilityLabel: "Ngõ 387", Overall: 5, Cleanliness: 5, Comment: "Sạch sẽ", CreatedAt: base},
		{ID: "r2", RoomID: e.roomID, RoomCode: "CG-TEST01", ListingName: "Căn thử nghiệm",
			FacilityID: 309, FacilityLabel: "Ngõ 387", Overall: 1, Cleanliness: 1, Comment: "Ko thay ga",
			AboutCleaning: true, CreatedAt: base - 2*day},
		{ID: "r3", RoomID: "phong-khac", RoomCode: "BĐ-X", ListingName: "Phòng khác",
			FacilityID: 147, FacilityLabel: "Cầu Giấy", Overall: 4, Cleanliness: 4, CreatedAt: base - 20*day},
	}
	if _, err := e.app.store.UpsertReviews(list, base); err != nil {
		t.Fatal(err)
	}
}

func (e *hkTestEnv) reviews(t *testing.T, query string) map[string]interface{} {
	t.Helper()
	w := e.do("GET", "/api/hk/reviews?"+query, e.adminToken, nil)
	if w.Code != 200 {
		t.Fatalf("lọc hỏng (%d): %s", w.Code, w.Body.String())
	}
	var out map[string]interface{}
	e.decode(w, &out)
	return out
}

func reviewIDs(out map[string]interface{}) []string {
	list, _ := out["reviews"].([]interface{})
	ids := []string{}
	for _, x := range list {
		if m, ok := x.(map[string]interface{}); ok {
			ids = append(ids, m["id"].(string))
		}
	}
	return ids
}

func TestReviewFilterByRoom(t *testing.T) {
	e := newHKTestEnv(t)
	e.seedReviews(t)
	got := reviewIDs(e.reviews(t, "room_id="+e.roomID+"&from=2000-01-01"))
	if len(got) != 2 {
		t.Fatalf("muốn 2 đánh giá của phòng đó, được %v", got)
	}
}

func TestReviewFilterByFacility(t *testing.T) {
	e := newHKTestEnv(t)
	e.seedReviews(t)
	got := reviewIDs(e.reviews(t, "facility_id=147&from=2000-01-01"))
	if len(got) != 1 || got[0] != "r3" {
		t.Fatalf("muốn đúng r3, được %v", got)
	}
}

func TestReviewFilterByStars(t *testing.T) {
	e := newHKTestEnv(t)
	e.seedReviews(t)
	got := reviewIDs(e.reviews(t, "stars=1&from=2000-01-01"))
	if len(got) != 1 || got[0] != "r2" {
		t.Fatalf("chip 1 sao phải ra đúng r2, được %v", got)
	}
	got = reviewIDs(e.reviews(t, "stars=5,4&from=2000-01-01"))
	if len(got) != 2 {
		t.Fatalf("chip 5+4 sao phải ra 2 đánh giá, được %v", got)
	}
}

// Khoảng ngày phải GỒM CẢ ngày cuối: người dùng chọn "đến 14/8" là muốn cả 14/8.
func TestReviewFilterDateRangeInclusive(t *testing.T) {
	e := newHKTestEnv(t)
	e.seedReviews(t)
	today := time.Now().Format("2006-01-02")
	got := reviewIDs(e.reviews(t, "from="+today+"&to="+today))
	if len(got) != 1 || got[0] != "r1" {
		t.Fatalf("đánh giá hôm nay phải nằm trong khoảng hôm nay→hôm nay, được %v", got)
	}
}

// Số trên mỗi chip sao KHÔNG đổi khi bấm chip khác — nếu đổi thì người dùng
// không bao giờ biết còn bao nhiêu đơn 1 sao.
func TestStarCountsIgnoreStarFilter(t *testing.T) {
	e := newHKTestEnv(t)
	e.seedReviews(t)
	a := e.reviews(t, "from=2000-01-01")["star_counts"].(map[string]interface{})
	b := e.reviews(t, "stars=5&from=2000-01-01")["star_counts"].(map[string]interface{})
	if len(a) != len(b) || a["1"] != b["1"] || a["5"] != b["5"] {
		t.Fatalf("số trên chip đổi khi lọc sao: %v vs %v", a, b)
	}
}

// Danh mục ô lọc chỉ gồm phòng/cơ sở THẬT SỰ có đánh giá.
func TestReviewFilterOptions(t *testing.T) {
	e := newHKTestEnv(t)
	e.seedReviews(t)
	out := e.reviews(t, "from=2000-01-01")
	rooms, _ := out["rooms"].([]interface{})
	facs, _ := out["facilities"].([]interface{})
	if len(rooms) != 2 {
		t.Fatalf("muốn 2 phòng có đánh giá, được %d", len(rooms))
	}
	if len(facs) != 2 {
		t.Fatalf("muốn 2 cơ sở, được %d", len(facs))
	}
}

// Cô dọn dẹp cũng xem và lọc được — đây là màn để cô tự cải thiện.
func TestCleanerCanFilterReviews(t *testing.T) {
	e := newHKTestEnv(t)
	e.seedReviews(t)
	w := e.do("GET", "/api/hk/reviews?stars=1&from=2000-01-01", e.lanToken, nil)
	if w.Code != 200 {
		t.Fatalf("cô phải xem được đánh giá: %d %s", w.Code, w.Body.String())
	}
}

// Nhưng chỉ quản lý mới được bấm tải đánh giá mới về (gọi ra Dayladau).
func TestReviewSyncAdminOnly(t *testing.T) {
	e := newHKTestEnv(t)
	if w := e.do("POST", "/api/hk/reviews/sync", e.lanToken, map[string]int{"days": 7}); w.Code != http.StatusForbidden {
		t.Fatalf("muốn 403 được %d", w.Code)
	}
}

// DB tạo bởi bản CŨ phải tự thêm cột mới khi mở bằng bản mới.
//
// Lỗi gặp thật: thêm cột cơ sở xong, máy dev chạy ngon vì DB mới tinh, còn máy
// đã test trước đó thì mọi truy vấn phòng hỏng với "no such column".
func TestOpenUpgradesOldDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "housekeeping.db")

	// Dựng bảng theo bản cũ: thiếu facility_id, facility_label, clean_time.
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Đúng schema bản trước (có NOT NULL DEFAULT ''), chỉ thiếu ba cột mới —
	// dựng bảng lỏng hơn thật thì test không chứng minh được gì.
	_, err = old.Exec(`CREATE TABLE hk_room (
		id TEXT PRIMARY KEY,
		listing_id TEXT NOT NULL DEFAULT '', code TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '', address TEXT NOT NULL DEFAULT '',
		zone TEXT NOT NULL DEFAULT '', room_type TEXT NOT NULL DEFAULT '',
		host_id TEXT NOT NULL DEFAULT '', host_name TEXT NOT NULL DEFAULT '',
		template_id TEXT NOT NULL DEFAULT '', door_note TEXT NOT NULL DEFAULT '',
		checkin_hour INTEGER NOT NULL DEFAULT 14, checkout_hour INTEGER NOT NULL DEFAULT 11,
		active INTEGER NOT NULL DEFAULT 1, synced_at INTEGER NOT NULL DEFAULT 0)`)
	if err != nil {
		t.Fatal(err)
	}
	old.Exec(`INSERT INTO hk_room (id, name, active) VALUES ('r-cu', 'Phòng cũ', 1)`)
	old.Close()

	store, err := OpenHKStore(path)
	if err != nil {
		t.Fatalf("mở DB cũ bằng bản mới phải nâng cấp được: %v", err)
	}
	defer store.Close()

	// Đọc được nghĩa là cột mới đã thêm, và dữ liệu cũ còn nguyên.
	room, err := store.RoomByID("r-cu")
	if err != nil {
		t.Fatalf("đọc phòng cũ sau nâng cấp: %v", err)
	}
	if room.Name != "Phòng cũ" {
		t.Fatalf("dữ liệu cũ phải giữ nguyên: %+v", room)
	}
	if room.CleanTime != 1 {
		t.Fatalf("cột mới phải có giá trị mặc định, được %d", room.CleanTime)
	}
}
