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
	"strings"
	"sync"
	"time"
)

type HKReview struct {
	ID          string `json:"id"`
	ListingID   string `json:"listing_id"`
	ListingName string `json:"listing_name"`
	RoomCode    string `json:"room_code"`
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
					ListingName: room.Name, RoomCode: room.Code,
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

// handleReviews — cả quản lý lẫn cô dọn dẹp đều xem được.
//
// Cô xem được là có chủ đích: mục tiêu là để cô biết chất lượng công việc của
// mình qua mắt khách, không phải để quản lý giữ riêng làm cơ sở khiển trách.
func (a *HKApp) handleReviews(w http.ResponseWriter, r *http.Request) {
	if _, err := a.hkAuthUser(r); err != nil {
		hkFailAuth(w, err)
		return
	}
	days := 30
	if v := strings.TrimSpace(r.URL.Query().Get("days")); v != "" {
		if n := hkParseInt64(v); n > 0 && n <= 365 {
			days = int(n)
		}
	}
	since := time.Now().AddDate(0, 0, -days).UnixMilli()

	reviews, err := a.hkCollectReviews(since, 30)
	if err != nil {
		hkFail(w, http.StatusBadGateway, "Không tải được đánh giá từ Dayladau.")
		return
	}
	hkWriteJSON(w, http.StatusOK, map[string]interface{}{
		"days":  days,
		"stats": hkSummarizeReviews(reviews),
	})
}
