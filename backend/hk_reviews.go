package main

// Đánh giá của khách — lấy từ API công khai của Dayladau.
//
// Mục đích: cô dọn dẹp thấy được kết quả việc mình làm. Điểm `cleanliness` là
// thước đo trực tiếp nhất; một review 1 sao kèm câu "home không thay ga" nói rõ
// phải sửa gì hơn bất kỳ chỉ số nội bộ nào.
//
// Endpoint /v1/listings/{id}/reviews mở công khai (không cần token), nên module
// này không phải giữ mật khẩu của ai.
//
// Có lọc riêng nhóm review "liên quan dọn dẹp": điểm sạch sẽ thấp, hoặc nội dung
// nhắc tới ga/bẩn/mùi/rác. Đưa cả trăm review chung chung cho cô đọc thì cô
// không đọc; đưa đúng cái nói về việc của cô thì có tác dụng.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HKReview struct {
	ID            string `json:"id"`
	ListingID     string `json:"listing_id"`
	ListingName   string `json:"listing_name"`
	RoomID        string `json:"room_id"`
	RoomCode      string `json:"room_code"`
	FacilityID    int    `json:"facility_id"`
	FacilityLabel string `json:"facility_label"`
	Overall     int    `json:"overall"`
	Cleanliness int    `json:"cleanliness"`
	Comment     string `json:"comment"`
	GuestName   string `json:"guest_name"`
	CreatedAt   int64  `json:"created_at"`
	// Nội dung nhắc thẳng tới chuyện dọn dẹp, hoặc điểm sạch sẽ thấp.
	AboutCleaning bool `json:"about_cleaning"`
}

type HKReviewStats struct {
	Total          int        `json:"total"`
	AvgOverall     float64    `json:"avg_overall"`
	AvgCleanliness float64    `json:"avg_cleanliness"`
	FiveStar       int        `json:"five_star"`
	LowClean       int        `json:"low_clean"` // điểm sạch sẽ <= 3
	AboutCleaning  int        `json:"about_cleaning"`
	Recent         []HKReview `json:"recent"`
	NeedAttention  []HKReview `json:"need_attention"`
}

// Từ khoá tiếng Việt chỉ chuyện dọn dẹp. Cố tình để không dấu lẫn có dấu vì khách
// gõ cả hai kiểu.
var hkCleaningWords = []string{
	"ga giường", "ga giuong", "thay ga", "chăn", "chan ", "gối", "goi ",
	"bẩn", "ban ", "dơ", "do ban", "sạch", "sach", "vệ sinh", "ve sinh",
	"dọn", "don dep", "mùi", "mui ", "hôi", "hoi ", "rác", "rac ", "bụi", "bui ",
	"tóc", "toc ", "khăn", "khan ", "ẩm mốc", "am moc", "mốc", "moc ",
}

func hkIsAboutCleaning(r HKReview) bool {
	if r.Cleanliness > 0 && r.Cleanliness <= 3 {
		return true
	}
	c := strings.ToLower(r.Comment)
	if c == "" {
		return false
	}
	for _, w := range hkCleaningWords {
		if strings.Contains(c, w) {
			return true
		}
	}
	return false
}

type hkRawReview struct {
	ID          string          `json:"id"`
	ListingID   string          `json:"listing_id"`
	ListingName string          `json:"listing_name"`
	Overall     int             `json:"overall"`
	Cleanliness int             `json:"cleanliness"`
	Comment     string          `json:"comment"`
	GuestName   string          `json:"guest_name"`
	IsAnonymous bool            `json:"is_anonymous"`
	CreatedAt   json.RawMessage `json:"created_at"`
}

// hkParseTimeMs đọc mốc thời gian dù API trả giây hay mili-giây.
func hkParseTimeMs(raw json.RawMessage) int64 {
	var n int64
	if err := json.Unmarshal(raw, &n); err != nil {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return 0
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UnixMilli()
		}
		return 0
	}
	if n > 0 && n < 1e12 { // giây → mili-giây
		return n * 1000
	}
	return n
}

func hkFetchListingReviews(client *http.Client, listingID string, limit int) ([]hkRawReview, error) {
	if limit <= 0 {
		limit = 20
	}
	url := fmt.Sprintf("https://api.dayladau.com/v1/listings/%s/reviews?limit=%d", listingID, limit)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reviews %s trả HTTP %d", listingID, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var out struct {
		Reviews []hkRawReview `json:"reviews"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Reviews, nil
}

// hkCollectReviews gom review của mọi phòng đang vận hành.
//
// Gọi song song nhưng giới hạn 6 kết nối cùng lúc: 60 phòng × request tuần tự là
// gần một phút chờ trắng màn, còn bắn 60 request một lúc thì dễ bị chặn.
func (a *HKApp) hkCollectReviews(sinceMs int64, perListing int) ([]HKReview, error) {
	rooms, err := a.store.ListRooms(true)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 25 * time.Second}

	var (
		mu   sync.Mutex
		all  []HKReview
		wg   sync.WaitGroup
		sem  = make(chan struct{}, 6)
		fail int
	)
	for _, room := range rooms {
		wg.Add(1)
		go func(room HKRoom) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			raws, err := hkFetchListingReviews(client, room.ListingID, perListing)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fail++
				return
			}
			for _, rr := range raws {
				ts := hkParseTimeMs(rr.CreatedAt)
				if sinceMs > 0 && ts > 0 && ts < sinceMs {
					continue
				}
				name := rr.GuestName
				if rr.IsAnonymous {
					name = "Khách ẩn danh"
				}
				rv := HKReview{
					ID: rr.ID, ListingID: rr.ListingID,
					ListingName: room.Name, RoomID: room.ID, RoomCode: room.Code,
					FacilityID: room.FacilityID, FacilityLabel: room.FacilityLabel,
					Overall: rr.Overall, Cleanliness: rr.Cleanliness,
					Comment: strings.TrimSpace(rr.Comment), GuestName: name, CreatedAt: ts,
				}
				rv.AboutCleaning = hkIsAboutCleaning(rv)
				all = append(all, rv)
			}
		}(room)
	}
	wg.Wait()

	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt > all[j].CreatedAt })
	return all, nil
}

func hkSummarizeReviews(reviews []HKReview) HKReviewStats {
	st := HKReviewStats{Recent: []HKReview{}, NeedAttention: []HKReview{}}
	var sumOverall, sumClean, nClean int
	for _, r := range reviews {
		st.Total++
		sumOverall += r.Overall
		if r.Cleanliness > 0 {
			sumClean += r.Cleanliness
			nClean++
			if r.Cleanliness <= 3 {
				st.LowClean++
			}
		}
		if r.Overall == 5 {
			st.FiveStar++
		}
		if r.AboutCleaning {
			st.AboutCleaning++
		}
	}
	if st.Total > 0 {
		st.AvgOverall = float64(sumOverall) / float64(st.Total)
	}
	if nClean > 0 {
		st.AvgCleanliness = float64(sumClean) / float64(nClean)
	}

	for _, r := range reviews {
		// Cần chú ý = điểm sạch sẽ thấp, hoặc lời chê có nhắc chuyện dọn dẹp.
		if r.Cleanliness > 0 && r.Cleanliness <= 3 || (r.AboutCleaning && r.Overall <= 3) {
			if len(st.NeedAttention) < 20 {
				st.NeedAttention = append(st.NeedAttention, r)
			}
		}
		if len(st.Recent) < 30 {
			st.Recent = append(st.Recent, r)
		}
	}
	return st
}

// hkSyncReviews kéo đánh giá từ Dayladau về cache.
func (a *HKApp) hkSyncReviews(days int) (int, error) {
	if days <= 0 {
		days = 180
	}
	since := time.Now().AddDate(0, 0, -days).UnixMilli()
	list, err := a.hkCollectReviews(since, 50)
	if err != nil {
		return 0, err
	}
	return a.store.UpsertReviews(list, hkNowMs())
}

func (a *HKApp) handleReviewsSync(w http.ResponseWriter, r *http.Request) {
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

	n, err := a.hkSyncReviews(body.Days)
	if err != nil {
		hkFail(w, http.StatusBadGateway, "Không tải được đánh giá từ Dayladau.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{"synced": n, "synced_at": hkNowMs()})
}

// hkParseStars đọc tham số chip sao dạng "5,4" → []int{5,4}. Bỏ giá trị ngoài 1..5.
func hkParseStars(v string) []int {
	out := []int{}
	for _, part := range strings.Split(v, ",") {
		n := hkParseInt64(part)
		if n >= 1 && n <= 5 {
			out = append(out, int(n))
		}
	}
	return out
}

// handleReviews — cả quản lý lẫn cô dọn dẹp đều xem được.
//
// Cô xem được là có chủ đích: mục tiêu là để cô biết chất lượng công việc của
// mình qua mắt khách, không phải để quản lý giữ riêng làm cơ sở khiển trách.
//
// Đọc từ cache trong DB chứ không gọi thẳng Dayladau: người dùng đổi bộ lọc liên
// tục, mỗi lần chờ vài giây thì không ai dùng bộ lọc nữa. Bấm "Tải đánh giá mới"
// mới đi lấy bản mới.
func (a *HKApp) handleReviews(w http.ResponseWriter, r *http.Request) {
	if _, err := a.hkAuthUser(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	q := r.URL.Query()
	loc := time.Now().Location()

	f := HKReviewFilter{
		RoomID:     strings.TrimSpace(q.Get("room_id")),
		FacilityID: int(hkParseInt64(q.Get("facility_id"))),
		Stars:      hkParseStars(q.Get("stars")),
	}
	// Khoảng ngày: `from`/`to` dạng YYYY-MM-DD. Không truyền thì lấy 30 ngày gần nhất.
	if v := strings.TrimSpace(q.Get("from")); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, loc); err == nil {
			f.From = t.UnixMilli()
		}
	}
	if v := strings.TrimSpace(q.Get("to")); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, loc); err == nil {
			// Hết ngày, không phải 00:00 — người dùng chọn "đến 14/8" là muốn gồm cả 14/8.
			f.To = t.AddDate(0, 0, 1).Add(-time.Millisecond).UnixMilli()
		}
	}
	if f.From == 0 && f.To == 0 {
		f.From = time.Now().AddDate(0, 0, -30).UnixMilli()
	}

	reviews, err := a.store.ListReviews(f)
	if err != nil {
		hkFail(w, http.StatusInternalServerError, "Không đọc được đánh giá.")
		return
	}

	// Danh mục cho ô lọc: chỉ những phòng/cơ sở THẬT SỰ có đánh giá, để người dùng
	// không chọn một mục rồi nhận về danh sách rỗng.
	all, _ := a.store.ListReviews(HKReviewFilter{})
	type opt struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	roomSeen, facSeen := map[string]bool{}, map[int]bool{}
	rooms, facilities := []opt{}, []opt{}
	for _, rv := range all {
		if rv.RoomID != "" && !roomSeen[rv.RoomID] {
			roomSeen[rv.RoomID] = true
			label := rv.ListingName
			if rv.RoomCode != "" {
				label = rv.RoomCode + " · " + label
			}
			rooms = append(rooms, opt{ID: rv.RoomID, Label: label})
		}
		if rv.FacilityID > 0 && !facSeen[rv.FacilityID] {
			facSeen[rv.FacilityID] = true
			label := rv.FacilityLabel
			if label == "" {
				label = fmt.Sprintf("Cơ sở #%d", rv.FacilityID)
			}
			facilities = append(facilities, opt{ID: strconv.Itoa(rv.FacilityID), Label: label})
		}
	}
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].Label < rooms[j].Label })
	sort.Slice(facilities, func(i, j int) bool { return facilities[i].Label < facilities[j].Label })

	// Đếm theo từng mức sao trên tập ĐÃ lọc phòng/cơ sở/ngày nhưng CHƯA lọc sao,
	// để con số trên mỗi chip không đổi khi bấm chip khác.
	countFilter := f
	countFilter.Stars = nil
	forCount, _ := a.store.ListReviews(countFilter)
	starCounts := map[string]int{}
	for _, rv := range forCount {
		if rv.Overall >= 1 && rv.Overall <= 5 {
			starCounts[strconv.Itoa(rv.Overall)]++
		}
	}

	hkWriteJSON(w, http.StatusOK, map[string]interface{}{
		"stats":        hkSummarizeReviews(reviews),
		"reviews":      reviews,
		"star_counts":  starCounts,
		"rooms":        rooms,
		"facilities":   facilities,
		"last_sync_at": a.store.LastReviewSync(),
	})
}
