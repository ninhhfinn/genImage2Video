package main

// Tầng lưu trữ module Dọn dẹp — SQLite (một file .db, không cần server DB).
//
// Toàn bộ truy cập DB đi qua *HKStore. Nếu sau này chuyển sang Postgres hoặc
// Firebase thì chỉ phải viết lại file này; handler và luật nghiệp vụ không đụng
// tới SQL.
//
// Driver là modernc.org/sqlite — bản thuần Go, KHÔNG cần cgo, nên `go build`
// vẫn chạy trên máy không có trình biên dịch C và cross-compile được như cũ.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type HKStore struct {
	db *sql.DB
	// SQLite cho phép nhiều reader nhưng một writer. Ghi đồng thời từ 5-6 cô cùng
	// lúc thì driver trả "database is locked" thay vì chờ, nên serialise ghi ở
	// đây cho chắc — quy mô này không cần hơn.
	writeMu sync.Mutex
}

func hkDataDir() string {
	if d := strings.TrimSpace(os.Getenv("HK_DATA_DIR")); d != "" {
		return d
	}
	return "hk_data"
}

func OpenHKStore(path string) (*HKStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil && filepath.Dir(path) != "." {
		return nil, err
	}
	// busy_timeout: chờ 5 giây thay vì lỗi ngay khi có người khác đang ghi.
	// foreign_keys: bật để phụ cấp không mồ côi khi ca bị xoá.
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &HKStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *HKStore) Close() error { return s.db.Close() }

func (s *HKStore) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS hk_user (
			id TEXT PRIMARY KEY,
			role TEXT NOT NULL,
			name TEXT NOT NULL,
			phone TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			status TEXT NOT NULL,
			zones TEXT NOT NULL DEFAULT '[]',
			note TEXT NOT NULL DEFAULT '',
			bank TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			reviewed_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS hk_token (
			token TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS hk_token_user ON hk_token(user_id)`,
		`CREATE TABLE IF NOT EXISTS hk_room (
			id TEXT PRIMARY KEY,
			listing_id TEXT NOT NULL DEFAULT '',
			code TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			address TEXT NOT NULL DEFAULT '',
			zone TEXT NOT NULL DEFAULT '',
			room_type TEXT NOT NULL DEFAULT '',
			host_id TEXT NOT NULL DEFAULT '',
			host_name TEXT NOT NULL DEFAULT '',
			template_id TEXT NOT NULL DEFAULT '',
			door_note TEXT NOT NULL DEFAULT '',
			clean_time INTEGER NOT NULL DEFAULT 1,
			facility_id INTEGER NOT NULL DEFAULT 0,
			facility_label TEXT NOT NULL DEFAULT '',
			checkin_hour INTEGER NOT NULL DEFAULT 14,
			checkout_hour INTEGER NOT NULL DEFAULT 11,
			active INTEGER NOT NULL DEFAULT 1,
			synced_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS hk_template (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			room_types TEXT NOT NULL DEFAULT '[]',
			groups TEXT NOT NULL DEFAULT '[]',
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS hk_session (
			id TEXT PRIMARY KEY,
			day TEXT NOT NULL,
			room_id TEXT NOT NULL,
			listing_id TEXT NOT NULL DEFAULT '',
			template_id TEXT NOT NULL DEFAULT '',
			staff_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			checkout_at INTEGER NOT NULL DEFAULT 0,
			next_checkin_at INTEGER NOT NULL DEFAULT 0,
			deadline_at INTEGER NOT NULL DEFAULT 0,
			guest_note TEXT NOT NULL DEFAULT '',
			booking_uid TEXT NOT NULL DEFAULT '',
			items_state TEXT NOT NULL DEFAULT '{}',
			template_snapshot TEXT NOT NULL DEFAULT '',
			started_at INTEGER NOT NULL DEFAULT 0,
			submitted_at INTEGER NOT NULL DEFAULT 0,
			reviewed_at INTEGER NOT NULL DEFAULT 0,
			reviewed_by TEXT NOT NULL DEFAULT '',
			review_note TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS hk_session_day ON hk_session(day)`,
		`CREATE INDEX IF NOT EXISTS hk_session_staff ON hk_session(staff_id)`,
		`CREATE INDEX IF NOT EXISTS hk_session_checkout ON hk_session(checkout_at)`,
		`CREATE TABLE IF NOT EXISTS hk_review (
			id TEXT PRIMARY KEY,
			listing_id TEXT NOT NULL DEFAULT '',
			room_id TEXT NOT NULL DEFAULT '',
			room_code TEXT NOT NULL DEFAULT '',
			room_name TEXT NOT NULL DEFAULT '',
			facility_id INTEGER NOT NULL DEFAULT 0,
			facility_label TEXT NOT NULL DEFAULT '',
			overall INTEGER NOT NULL DEFAULT 0,
			cleanliness INTEGER NOT NULL DEFAULT 0,
			comment TEXT NOT NULL DEFAULT '',
			guest_name TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0,
			about_cleaning INTEGER NOT NULL DEFAULT 0,
			synced_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS hk_review_created ON hk_review(created_at)`,
		`CREATE INDEX IF NOT EXISTS hk_review_room ON hk_review(room_id)`,
		`CREATE INDEX IF NOT EXISTS hk_review_facility ON hk_review(facility_id)`,
		// Một phòng chỉ có một ca mỗi ngày — chặn ở tầng DB để job đồng bộ chạy hai
		// lần không đẻ ra hai ca, kéo theo trả tiền hai lần.
		// Khoá chống trùng là MÃ LƯỢT ĐẶT, không phải (phòng, ngày): 59/60 phòng
		// cho thuê theo giờ nên một phòng có nhiều lượt khách trong một ngày, mỗi
		// lượt phải là một ca dọn kỹ riêng.
		`CREATE UNIQUE INDEX IF NOT EXISTS hk_session_booking ON hk_session(booking_uid)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w (%s)", err, firstLineOf(q))
		}
	}
	return s.addMissingColumns()
}

// addMissingColumns thêm cột mới vào bảng ĐÃ TỒN TẠI.
//
// `CREATE TABLE IF NOT EXISTS` chỉ tạo bảng lần đầu; máy nào đã chạy bản cũ thì
// bảng vẫn thiếu cột mới và mọi truy vấn hỏng với "no such column". Đây là lỗi
// gặp thật khi thêm cột cơ sở: máy dev vẫn chạy vì DB mới, còn máy đã test thì
// hỏng ngay.
//
// SQLite không có "ADD COLUMN IF NOT EXISTS" nên phải hỏi PRAGMA trước.
func (s *HKStore) addMissingColumns() error {
	wanted := map[string]map[string]string{
		"hk_room": {
			"clean_time":     "INTEGER NOT NULL DEFAULT 1",
			"facility_id":    "INTEGER NOT NULL DEFAULT 0",
			"facility_label": "TEXT NOT NULL DEFAULT ''",
		},
		"hk_session": {
			"booking_uid": "TEXT NOT NULL DEFAULT ''",
		},
	}
	for table, cols := range wanted {
		have, err := s.columnsOf(table)
		if err != nil {
			return err
		}
		if len(have) == 0 {
			continue // bảng chưa tồn tại; phần CREATE ở trên lo
		}
		for name, decl := range cols {
			if have[name] {
				continue
			}
			stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, name, decl)
			if _, err := s.db.Exec(stmt); err != nil {
				return fmt.Errorf("thêm cột %s.%s: %w", table, name, err)
			}
		}
	}
	return nil
}

func (s *HKStore) columnsOf(table string) (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = true
	}
	return out, rows.Err()
}

func firstLineOf(q string) string {
	if i := strings.IndexByte(q, '\n'); i > 0 {
		return q[:i]
	}
	return q
}

func jsonMarshalString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// ─── Người dùng ───────────────────────────────────────────────────────────

func (s *HKStore) UpsertUser(u HKUser) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO hk_user (id, role, name, phone, password_hash, status, zones, note, bank, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			role=excluded.role, name=excluded.name, phone=excluded.phone,
			password_hash=excluded.password_hash, status=excluded.status,
			zones=excluded.zones, note=excluded.note, bank=excluded.bank`,
		u.ID, u.Role, u.Name, hkNormalizePhone(u.Phone), u.PasswordHash, u.Status,
		jsonMarshalString(u.Zones), u.Note, u.Bank, u.CreatedAt)
	return err
}

func scanUser(row interface{ Scan(...interface{}) error }) (HKUser, error) {
	var u HKUser
	var zones string
	err := row.Scan(&u.ID, &u.Role, &u.Name, &u.Phone, &u.PasswordHash, &u.Status,
		&zones, &u.Note, &u.Bank, &u.CreatedAt)
	if err != nil {
		return u, err
	}
	json.Unmarshal([]byte(zones), &u.Zones)
	if u.Zones == nil {
		u.Zones = []string{}
	}
	return u, nil
}

const userCols = `id, role, name, phone, password_hash, status, zones, note, bank, created_at`

func (s *HKStore) UserByPhone(phone string) (HKUser, error) {
	row := s.db.QueryRow(`SELECT `+userCols+` FROM hk_user WHERE phone = ?`, hkNormalizePhone(phone))
	return scanUser(row)
}

func (s *HKStore) UserByID(id string) (HKUser, error) {
	row := s.db.QueryRow(`SELECT `+userCols+` FROM hk_user WHERE id = ?`, id)
	return scanUser(row)
}

func (s *HKStore) ListUsers(role string) ([]HKUser, error) {
	q := `SELECT ` + userCols + ` FROM hk_user`
	var args []interface{}
	if role != "" {
		q += ` WHERE role = ?`
		args = append(args, role)
	}
	q += ` ORDER BY name COLLATE NOCASE`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HKUser{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *HKStore) SetUserStatus(id, status string, now int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`UPDATE hk_user SET status=?, reviewed_at=? WHERE id=?`, status, now, id)
	return err
}

// ─── Token phiên ──────────────────────────────────────────────────────────

func (s *HKStore) SaveToken(token, userID string, now, expires int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`INSERT INTO hk_token (token, user_id, created_at, expires_at) VALUES (?,?,?,?)`,
		token, userID, now, expires)
	return err
}

func (s *HKStore) UserByToken(token string, now int64) (HKUser, error) {
	row := s.db.QueryRow(`
		SELECT u.`+strings.ReplaceAll(userCols, ", ", ", u.")+`
		FROM hk_token t JOIN hk_user u ON u.id = t.user_id
		WHERE t.token = ? AND t.expires_at > ?`, token, now)
	return scanUser(row)
}

func (s *HKStore) DeleteToken(token string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`DELETE FROM hk_token WHERE token = ?`, token)
	return err
}

// PurgeExpiredTokens dọn token hết hạn. Không có nó thì bảng phình vô hạn vì mỗi
// lần đăng nhập là một dòng mới.
func (s *HKStore) PurgeExpiredTokens(now int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`DELETE FROM hk_token WHERE expires_at <= ?`, now)
	return err
}

// ─── Phòng ────────────────────────────────────────────────────────────────

const roomCols = `id, listing_id, code, name, address, zone, room_type, host_id, host_name,
	template_id, door_note, clean_time, facility_id, facility_label, checkin_hour, checkout_hour, active, synced_at`

func scanRoom(row interface{ Scan(...interface{}) error }) (HKRoom, error) {
	var r HKRoom
	var active int
	err := row.Scan(&r.ID, &r.ListingID, &r.Code, &r.Name, &r.Address, &r.Zone, &r.RoomType,
		&r.HostID, &r.HostName, &r.TemplateID, &r.DoorNote, &r.CleanTime, &r.FacilityID, &r.FacilityLabel,
		&r.CheckinHr, &r.CheckoutHr, &active, &r.SyncedAt)
	r.Active = active != 0
	return r, err
}

// UpsertRoom giữ nguyên các trường do người dùng chỉnh tay (đơn giá, mẫu
// checklist, hướng dẫn vào nhà) khi đồng bộ lại từ Dayladau — nếu không, mỗi lần
// bấm Đồng bộ là quản lý mất hết công sửa.
func (s *HKStore) UpsertRoom(r HKRoom) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	active := 0
	if r.Active {
		active = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO hk_room (`+roomCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			listing_id=excluded.listing_id, code=excluded.code, name=excluded.name,
			address=excluded.address, zone=excluded.zone, room_type=excluded.room_type,
			host_id=excluded.host_id, host_name=excluded.host_name,
			checkin_hour=excluded.checkin_hour, checkout_hour=excluded.checkout_hour,
			facility_id=excluded.facility_id, facility_label=excluded.facility_label,
			active=excluded.active, synced_at=excluded.synced_at`,
		r.ID, r.ListingID, r.Code, r.Name, r.Address, r.Zone, r.RoomType, r.HostID, r.HostName,
		r.TemplateID, r.DoorNote, r.CleanTime, r.FacilityID, r.FacilityLabel, r.CheckinHr, r.CheckoutHr, active, r.SyncedAt)
	return err
}

func (s *HKStore) UpdateRoomSettings(id, templateID, doorNote string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`UPDATE hk_room SET template_id=?, door_note=? WHERE id=?`,
		templateID, doorNote, id)
	return err
}

func (s *HKStore) ListRooms(onlyActive bool) ([]HKRoom, error) {
	q := `SELECT ` + roomCols + ` FROM hk_room`
	if onlyActive {
		q += ` WHERE active = 1`
	}
	q += ` ORDER BY zone, name`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HKRoom{}
	for rows.Next() {
		r, err := scanRoom(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *HKStore) RoomByID(id string) (HKRoom, error) {
	return scanRoom(s.db.QueryRow(`SELECT `+roomCols+` FROM hk_room WHERE id = ?`, id))
}

// ─── Mẫu checklist ────────────────────────────────────────────────────────

func (s *HKStore) UpsertTemplate(t HKTemplate) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`
		INSERT INTO hk_template (id, name, room_types, groups, updated_at) VALUES (?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, room_types=excluded.room_types,
			groups=excluded.groups, updated_at=excluded.updated_at`,
		t.ID, t.Name, jsonMarshalString(t.RoomTypes), jsonMarshalString(t.Groups), t.UpdatedAt)
	return err
}

func scanTemplate(row interface{ Scan(...interface{}) error }) (HKTemplate, error) {
	var t HKTemplate
	var roomTypes, groups string
	if err := row.Scan(&t.ID, &t.Name, &roomTypes, &groups, &t.UpdatedAt); err != nil {
		return t, err
	}
	json.Unmarshal([]byte(roomTypes), &t.RoomTypes)
	json.Unmarshal([]byte(groups), &t.Groups)
	if t.RoomTypes == nil {
		t.RoomTypes = []string{}
	}
	if t.Groups == nil {
		t.Groups = []HKGroup{}
	}
	return t, nil
}

func (s *HKStore) ListTemplates() ([]HKTemplate, error) {
	rows, err := s.db.Query(`SELECT id, name, room_types, groups, updated_at FROM hk_template ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HKTemplate{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *HKStore) TemplateByID(id string) (HKTemplate, error) {
	return scanTemplate(s.db.QueryRow(
		`SELECT id, name, room_types, groups, updated_at FROM hk_template WHERE id = ?`, id))
}

// ─── Ca dọn ───────────────────────────────────────────────────────────────

const sessionCols = `id, day, room_id, listing_id, template_id, staff_id, status,
	checkout_at, next_checkin_at, deadline_at, guest_note, booking_uid,
	items_state, template_snapshot, started_at, submitted_at, reviewed_at, reviewed_by, review_note`

func scanSession(row interface{ Scan(...interface{}) error }) (HKSession, error) {
	var s HKSession
	var itemsState, snapshot string
	err := row.Scan(&s.ID, &s.Day, &s.RoomID, &s.ListingID, &s.TemplateID, &s.StaffID, &s.Status,
		&s.CheckoutAt, &s.NextCheckinAt, &s.DeadlineAt, &s.GuestNote, &s.BookingUID,
		&itemsState, &snapshot, &s.StartedAt, &s.SubmittedAt, &s.ReviewedAt, &s.ReviewedBy, &s.ReviewNote)
	if err != nil {
		return s, err
	}
	json.Unmarshal([]byte(itemsState), &s.ItemsState)
	if s.ItemsState == nil {
		s.ItemsState = map[string]HKItemState{}
	}
	if strings.TrimSpace(snapshot) != "" {
		var t HKTemplate
		if json.Unmarshal([]byte(snapshot), &t) == nil {
			s.TemplateSnapshot = &t
		}
	}
	return s, nil
}

// InsertSessionIfAbsent trả về false khi phòng đó đã có ca trong ngày. Dùng
// INSERT OR IGNORE dựa trên unique index (room_id, day) thay vì SELECT-rồi-INSERT
// để hai lần đồng bộ chạy song song không cùng lọt qua khe kiểm tra.
func (s *HKStore) InsertSessionIfAbsent(sess HKSession) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO hk_session (`+sessionCols+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sess.ID, sess.Day, sess.RoomID, sess.ListingID, sess.TemplateID, sess.StaffID, sess.Status,
		sess.CheckoutAt, sess.NextCheckinAt, sess.DeadlineAt, sess.GuestNote, sess.BookingUID,
		jsonMarshalString(sess.ItemsState), snapshotJSON(sess.TemplateSnapshot),
		sess.StartedAt, sess.SubmittedAt, sess.ReviewedAt, sess.ReviewedBy, sess.ReviewNote)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func snapshotJSON(t *HKTemplate) string {
	if t == nil {
		return ""
	}
	return jsonMarshalString(t)
}

// ListUnassignedSessions — ca chưa có người phụ trách và chưa ai động vào, trong
// một khoảng ngày. Dùng để gợi ý lại người khi đồng bộ chạy lần sau.
func (s *HKStore) ListUnassignedSessions(fromDay, toDay string) ([]HKSession, error) {
	rows, err := s.db.Query(`SELECT `+sessionCols+`
		FROM hk_session
		WHERE staff_id = '' AND started_at = 0 AND day >= ? AND day <= ?
		ORDER BY checkout_at`, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HKSession{}
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// AssignSessionStaff chỉ đổi người phụ trách, không đụng các trường khác.
func (s *HKStore) AssignSessionStaff(id, staffID string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`UPDATE hk_session SET staff_id = ? WHERE id = ?`, staffID, id)
	return err
}

func (s *HKStore) UpdateSession(sess HKSession) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`
		UPDATE hk_session SET staff_id=?, status=?, guest_note=?,
			items_state=?, started_at=?, submitted_at=?, reviewed_at=?, reviewed_by=?, review_note=?
		WHERE id=?`,
		sess.StaffID, sess.Status, sess.GuestNote,
		jsonMarshalString(sess.ItemsState), sess.StartedAt, sess.SubmittedAt,
		sess.ReviewedAt, sess.ReviewedBy, sess.ReviewNote, sess.ID)
	return err
}

type HKSessionFilter struct {
	Day     string
	From    int64
	To      int64
	StaffID string
	Status  string
}

func (s *HKStore) ListSessions(f HKSessionFilter) ([]HKSession, error) {
	q := `SELECT ` + sessionCols + ` FROM hk_session WHERE 1=1`
	var args []interface{}
	if f.Day != "" {
		q += ` AND day = ?`
		args = append(args, f.Day)
	}
	if f.From > 0 {
		q += ` AND checkout_at >= ?`
		args = append(args, f.From)
	}
	if f.To > 0 {
		q += ` AND checkout_at <= ?`
		args = append(args, f.To)
	}
	if f.StaffID != "" {
		q += ` AND staff_id = ?`
		args = append(args, f.StaffID)
	}
	if f.Status != "" {
		q += ` AND status = ?`
		args = append(args, f.Status)
	}
	q += ` ORDER BY checkout_at`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HKSession{}
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func (s *HKStore) SessionByID(id string) (HKSession, error) {
	sess, err := scanSession(s.db.QueryRow(`SELECT `+sessionCols+` FROM hk_session WHERE id = ?`, id))
	return sess, err
}

func (s *HKStore) CountRows(table string) (int, error) {
	// Chỉ nhận tên bảng nội bộ — không nhận đầu vào từ người dùng.
	switch table {
	case "hk_user", "hk_room", "hk_template", "hk_session":
	default:
		return 0, fmt.Errorf("bảng không hợp lệ: %s", table)
	}
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
	return n, err
}

func hkNowMs() int64 { return time.Now().UnixMilli() }

// ─── Đánh giá của khách (cache) ───────────────────────────────────────────
//
// Lưu lại thay vì mỗi lần lọc lại gọi 60 request sang Dayladau: người dùng đổi
// bộ lọc liên tục, và chờ vài giây mỗi lần bấm thì không ai dùng bộ lọc nữa.

func (s *HKStore) UpsertReviews(list []HKReview, now int64) (int, error) {
	if len(list) == 0 {
		return 0, nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT INTO hk_review (id, listing_id, room_id, room_code, room_name, facility_id, facility_label,
			overall, cleanliness, comment, guest_name, created_at, about_cleaning, synced_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			room_id=excluded.room_id, room_code=excluded.room_code, room_name=excluded.room_name,
			facility_id=excluded.facility_id, facility_label=excluded.facility_label,
			overall=excluded.overall, cleanliness=excluded.cleanliness, comment=excluded.comment,
			about_cleaning=excluded.about_cleaning, synced_at=excluded.synced_at`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	n := 0
	for _, r := range list {
		about := 0
		if r.AboutCleaning {
			about = 1
		}
		if _, err := stmt.Exec(r.ID, r.ListingID, r.RoomID, r.RoomCode, r.ListingName,
			r.FacilityID, r.FacilityLabel, r.Overall, r.Cleanliness, r.Comment,
			r.GuestName, r.CreatedAt, about, now); err != nil {
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

type HKReviewFilter struct {
	RoomID     string
	FacilityID int
	From       int64
	To         int64
	Stars      []int // rỗng = mọi mức sao
}

func (s *HKStore) ListReviews(f HKReviewFilter) ([]HKReview, error) {
	q := `SELECT id, listing_id, room_id, room_code, room_name, facility_id, facility_label,
		overall, cleanliness, comment, guest_name, created_at, about_cleaning
		FROM hk_review WHERE 1=1`
	var args []interface{}
	if f.RoomID != "" {
		q += ` AND room_id = ?`
		args = append(args, f.RoomID)
	}
	if f.FacilityID > 0 {
		q += ` AND facility_id = ?`
		args = append(args, f.FacilityID)
	}
	if f.From > 0 {
		q += ` AND created_at >= ?`
		args = append(args, f.From)
	}
	if f.To > 0 {
		q += ` AND created_at <= ?`
		args = append(args, f.To)
	}
	if len(f.Stars) > 0 {
		// Chip sao lọc theo ĐIỂM CHUNG (overall) — đó là con số khách nhìn thấy và
		// là con số quản lý nói tới khi bảo "đơn 1 sao".
		q += ` AND overall IN (` + strings.TrimSuffix(strings.Repeat("?,", len(f.Stars)), ",") + `)`
		for _, st := range f.Stars {
			args = append(args, st)
		}
	}
	q += ` ORDER BY created_at DESC`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HKReview{}
	for rows.Next() {
		var r HKReview
		var about int
		if err := rows.Scan(&r.ID, &r.ListingID, &r.RoomID, &r.RoomCode, &r.ListingName,
			&r.FacilityID, &r.FacilityLabel, &r.Overall, &r.Cleanliness, &r.Comment,
			&r.GuestName, &r.CreatedAt, &about); err != nil {
			return nil, err
		}
		r.AboutCleaning = about != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// LastReviewSync — lần đồng bộ đánh giá gần nhất, để màn hình nói rõ số liệu cũ
// tới mức nào thay vì để người dùng đoán.
func (s *HKStore) LastReviewSync() int64 {
	var n int64
	s.db.QueryRow(`SELECT COALESCE(MAX(synced_at), 0) FROM hk_review`).Scan(&n)
	return n
}
