package main

// Doanh thu theo NGÀY CHECK-IN.
//
// Khác biệt cốt lõi so với báo cáo của host.dayladau.com: báo cáo bên đó tính
// theo ngày BÁN ĐƠN, còn vận hành dọn dẹp quan tâm ngày khách THẬT SỰ Ở. Một đơn
// đặt hôm nay cho ngày ở tháng sau rơi vào hai ngày khác nhau tuỳ cách tính. Đo
// thật trên 30 ngày dữ liệu Unixstay: 257.745.785đ theo check-in so với
// 263.430.363đ theo ngày bán đơn — lệch 2,2%.
//
// May mắn là API Dayladau đã hỗ trợ sẵn `timeline=checkin`, nên không phải tự
// cộng lại từ đơn thô.
//
// KHÁC MỌI PHẦN CÒN LẠI CỦA MODULE: endpoint doanh thu ĐÒI TOKEN của tài khoản
// host. Token đọc từ file/biến môi trường, không bao giờ nằm trong mã nguồn.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── Token ────────────────────────────────────────────────────────────────

// hkDayladauToken đọc token theo thứ tự: biến môi trường → file.
//
// Mặc định đọc ~/.dayladau_token vì đó là chỗ lệnh đăng nhập ghi ra. Trả rỗng
// khi không có — chỗ gọi phải hiện thông báo rõ ràng chứ không im lặng trả 0đ,
// vì "doanh thu 0đ" và "chưa cấu hình token" là hai chuyện hoàn toàn khác nhau.
func hkDayladauToken() string {
	if t := strings.TrimSpace(os.Getenv("DLD_TOKEN")); t != "" {
		return t
	}
	path := strings.TrimSpace(os.Getenv("DLD_TOKEN_FILE"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(home, ".dayladau_token")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// hkHostID — tài khoản host sở hữu các phòng. Lấy từ phản hồi API (trường
// filter.host_id) nên không phải khai cứng.
func hkHostID() string {
	return strings.TrimSpace(os.Getenv("DLD_HOST_ID"))
}

// ─── Gọi API ──────────────────────────────────────────────────────────────

type hkRevenuePoint struct {
	Timestamp int64   `json:"timestamp"`
	Revenue   float64 `json:"revenue"`
	Total     int     `json:"total"`
}

type hkRevenueResponse struct {
	Filter struct {
		HostID string `json:"host_id"`
	} `json:"filter"`
	Data map[string][]hkRevenuePoint `json:"data"`
}

var errNoToken = fmt.Errorf("chưa cấu hình token Dayladau")

// hkFetchRevenue gọi báo cáo doanh thu cho một khoảng ngày.
//
// listingID rỗng = tổng của cả tài khoản host.
func hkFetchRevenue(client *http.Client, token, listingID string, from, to time.Time) ([]hkRevenuePoint, string, error) {
	if token == "" {
		return nil, "", errNoToken
	}
	u := fmt.Sprintf(
		"https://api.dayladau.com/v1/reports/revenue?from=%d&to=%d&timeline=checkin&group_by=listing_id&listing_id=%s&x_access_token=%s",
		from.UnixMilli(), to.UnixMilli(), listingID, token)

	resp, err := client.Get(u)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
		// Token hết hạn là chuyện sẽ xảy ra, không phải lỗi lạ — nói thẳng ra để
		// người dùng biết phải đăng nhập lại, thay vì thấy bảng trống.
		return nil, "", fmt.Errorf("token Dayladau không dùng được nữa (HTTP %d) — đăng nhập lại để lấy token mới", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("báo cáo doanh thu trả HTTP %d", resp.StatusCode)
	}

	var out hkRevenueResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, "", fmt.Errorf("không đọc được phản hồi doanh thu: %w", err)
	}
	// API gộp mọi nhóm vào một khoá (thường là "unknown") kể cả khi group_by=
	// listing_id, nên phải gộp hết các nhóm lại thay vì đọc một khoá cố định.
	var points []hkRevenuePoint
	for _, v := range out.Data {
		points = append(points, v...)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Timestamp < points[j].Timestamp })
	return points, out.Filter.HostID, nil
}

// ─── Đồng bộ vào cache ────────────────────────────────────────────────────

// hkSyncRevenue kéo doanh thu từng phòng về DB.
//
// Gọi song song nhưng giới hạn 5 kết nối: 54 phòng × request tuần tự là gần một
// phút chờ, còn bắn hết một lúc thì dễ bị chặn.
func (a *HKApp) hkSyncRevenue(days int) (int, error) {
	token := hkDayladauToken()
	if token == "" {
		return 0, errNoToken
	}
	if days <= 0 {
		days = 60
	}
	loc := time.Now().Location()
	to := time.Now().In(loc)
	from := to.AddDate(0, 0, -days)

	rooms, err := a.store.ListRooms(true)
	if err != nil {
		return 0, err
	}
	if len(rooms) == 0 {
		return 0, fmt.Errorf("chưa có phòng nào — bấm Đồng bộ phòng trước")
	}

	client := &http.Client{Timeout: 40 * time.Second}
	var (
		mu     sync.Mutex
		rows   []HKRevenueRow
		wg     sync.WaitGroup
		sem    = make(chan struct{}, 5)
		failed int
		fatal  error
	)
	for _, room := range rooms {
		wg.Add(1)
		go func(room HKRoom) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			points, _, err := hkFetchRevenue(client, token, room.ListingID, from, to)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				if strings.Contains(err.Error(), "token") {
					fatal = err
				}
				return
			}
			for _, p := range points {
				rows = append(rows, HKRevenueRow{
					RoomID:    room.ID,
					ListingID: room.ListingID,
					Day:       time.UnixMilli(p.Timestamp).In(loc).Format("2006-01-02"),
					Revenue:   int64(p.Revenue + 0.5),
					Bookings:  p.Total,
				})
			}
		}(room)
	}
	wg.Wait()

	// Token hỏng thì mọi phòng đều hỏng — báo đúng nguyên nhân thay vì "0 dòng".
	if fatal != nil && len(rows) == 0 {
		return 0, fatal
	}
	if failed > 0 {
		fmt.Printf("[hk] đồng bộ doanh thu: %d/%d phòng lỗi\n", failed, len(rooms))
	}
	return a.store.UpsertRevenue(rows, hkNowMs())
}

// ─── HTTP ─────────────────────────────────────────────────────────────────

func (a *HKApp) handleRevenueSync(w http.ResponseWriter, r *http.Request) {
	if !hkRequirePost(w, r) {
		return
	}
	if _, err := a.hkRequireAdmin(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	var body struct {
		Days int `json:"days"`
	}
	hkDecodeBody(r, &body)

	n, err := a.hkSyncRevenue(body.Days)
	if err != nil {
		if err == errNoToken {
			hkFail(w, http.StatusPreconditionFailed,
				"Chưa cấu hình token Dayladau. Đăng nhập tài khoản host rồi lưu token vào ~/.dayladau_token.")
			return
		}
		hkFail(w, http.StatusBadGateway, err.Error())
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"synced": n, "synced_at": hkNowMs()})
}

// handleRevenue — CHỈ quản lý. Doanh thu phòng là thông tin kinh doanh, và lương
// của cô dọn dẹp không liên quan gì tới nó.
func (a *HKApp) handleRevenue(w http.ResponseWriter, r *http.Request) {
	if _, err := a.hkRequireAdmin(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	loc := time.Now().Location()
	q := r.URL.Query()

	f := HKRevenueFilter{
		RoomID:     strings.TrimSpace(q.Get("room_id")),
		FacilityID: int(hkParseInt64(q.Get("facility_id"))),
	}
	if v := strings.TrimSpace(q.Get("from")); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, loc); err == nil {
			f.FromDay = t.Format("2006-01-02")
		}
	}
	if v := strings.TrimSpace(q.Get("to")); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, loc); err == nil {
			f.ToDay = t.Format("2006-01-02")
		}
	}
	if f.FromDay == "" && f.ToDay == "" {
		f.FromDay = time.Now().AddDate(0, 0, -29).Format("2006-01-02")
		f.ToDay = time.Now().Format("2006-01-02")
	}

	byDay, byRoom, total, err := a.store.RevenueSummary(f)
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không đọc được dữ liệu doanh thu.")
		return
	}

	hkWriteJSON(w, http.StatusOK, map[string]interface{}{
		"by_day":       byDay,
		"by_room":      byRoom,
		"total":        total,
		"has_token":    hkDayladauToken() != "",
		"last_sync_at": a.store.LastRevenueSync(),
		"from":         f.FromDay,
		"to":           f.ToDay,
	})
}

// hkFetchHostID hỏi Dayladau xem token này là tài khoản nào.
//
// Không khai cứng host_id trong mã: đổi tài khoản chỉ cần thay token, và nếu
// khai sai thì hệ thống lặng lẽ quản lý phòng của người khác.
func hkFetchHostID(token string) (string, error) {
	if id := hkHostID(); id != "" {
		return id, nil
	}
	if token == "" {
		return "", errNoToken
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get("https://api.dayladau.com/v1/me?x_access_token=" + token)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("/v1/me trả HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var me struct {
		ID     string `json:"id"`
		IsHost bool   `json:"is_host"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		return "", err
	}
	if me.ID == "" {
		return "", fmt.Errorf("không đọc được id tài khoản")
	}
	if !me.IsHost {
		return "", fmt.Errorf("tài khoản này không phải host — không đọc được danh sách phòng")
	}
	return me.ID, nil
}
