package main

// Đọc lịch đặt phòng từ feed iCal của Dayladau.
//
// Vì sao iCal chứ không phải /v2/calendars/search: endpoint đó đòi access_token
// của tài khoản host, còn feed iCal (https://api.dayladau.com/v1/listings/{id}/ical)
// mở công khai — chính feed mà host dán sang Airbnb/Booking để đồng bộ lịch. Nhờ
// vậy hệ thống này không phải giữ mật khẩu hay token của ai.
//
// ĐÁNH ĐỔI phải biết: iCal chỉ cho NGÀY, không cho GIỜ
// (DTSTART;VALUE=DATE:20260813). Với phòng cho thuê theo giờ — 59/60 phòng của
// Dayladau — hai lượt khách trong cùng một ngày trông giống hệt nhau, không biết
// lượt nào trả phòng lúc mấy giờ. Giờ hiển thị vì thế là giờ trả phòng chuẩn của
// căn đó, và quản lý sửa lại được trên màn điều phối. Muốn giờ chính xác thì phải
// đấu /v2/calendars/search kèm token — xem ghi chú ở hkFetchCheckouts().

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type hkICalEvent struct {
	UID     string
	Start   time.Time // ngày khách nhận phòng
	End     time.Time // ngày khách trả phòng (iCal DTEND của sự kiện cả ngày là ngày rời đi)
	Summary string
	Blocked bool // chủ nhà tự khoá lịch — KHÔNG có khách nên không phải dọn
}

// hkFetchICal tải và phân tích feed iCal của một listing.
func hkFetchICal(client *http.Client, listingID string, loc *time.Location) ([]hkICalEvent, error) {
	url := fmt.Sprintf("https://api.dayladau.com/v1/listings/%s/ical", listingID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iCal %s trả HTTP %d", listingID, resp.StatusCode)
	}
	// 1MB thừa sức cho một năm lịch; chặn để một feed lỗi không nuốt hết bộ nhớ.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return hkParseICal(string(body), loc)
}

// hkParseICal phân tích nội dung iCal. Tách riêng khỏi phần tải để test được mà
// không cần mạng.
func hkParseICal(body string, loc *time.Location) ([]hkICalEvent, error) {
	if loc == nil {
		loc = time.Local
	}
	var out []hkICalEvent
	var cur *hkICalEvent
	inEvent := false

	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case line == "BEGIN:VEVENT":
			inEvent = true
			cur = &hkICalEvent{}
			continue
		case line == "END:VEVENT":
			if cur != nil && !cur.End.IsZero() {
				// Dayladau ghi "Reserved for dayladau" cho khách thật và
				// "Blocked by dayladau" cho lượt chủ nhà tự khoá. Khoá lịch nghĩa
				// là không có ai ở — dọn ở đó là trả công cho việc không tồn tại.
				cur.Blocked = strings.Contains(strings.ToLower(cur.Summary), "block")
				out = append(out, *cur)
			}
			inEvent = false
			cur = nil
			continue
		}
		if !inEvent || cur == nil {
			continue
		}

		name, value := hkSplitICalLine(line)
		switch {
		case strings.HasPrefix(name, "DTSTART"):
			if t, ok := hkParseICalDate(value, loc); ok {
				cur.Start = t
			}
		case strings.HasPrefix(name, "DTEND"):
			if t, ok := hkParseICalDate(value, loc); ok {
				cur.End = t
			}
		case name == "UID":
			cur.UID = value
		case name == "SUMMARY":
			cur.Summary = value
		}
	}
	return out, sc.Err()
}

// hkSplitICalLine tách "DTSTART;VALUE=DATE:20260813" thành tên và giá trị. Tên
// giữ cả phần tham số vì chỗ gọi chỉ so bằng tiền tố.
func hkSplitICalLine(line string) (name, value string) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return line, ""
	}
	return line[:i], line[i+1:]
}

func hkParseICalDate(v string, loc *time.Location) (time.Time, bool) {
	v = strings.TrimSpace(v)
	// Dạng cả ngày: 20260813
	if len(v) == 8 {
		t, err := time.ParseInLocation("20060102", v, loc)
		return t, err == nil
	}
	// Dạng có giờ: 20260813T110000Z hoặc 20260813T110000
	for _, layout := range []string{"20060102T150405Z", "20060102T150405"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.In(loc), true
		}
		if t, err := time.ParseInLocation(layout, v, loc); err == nil {
			return t.In(loc), true
		}
	}
	return time.Time{}, false
}

// hkTurn — một lượt khách rời phòng, tức một ca dọn cần làm.
type hkTurn struct {
	RoomID     string
	ListingID  string
	UID        string    // mã lượt đặt, dùng làm khoá chống trùng
	Day        string    // YYYY-MM-DD ngày trả phòng
	CheckoutAt time.Time // giờ trả phòng (suy từ giờ chuẩn của căn)
	DeadlineAt time.Time // hạn dọn xong
	HasNext    bool      // có khách nhận phòng ngay trong ngày không
}

// hkTurnsFromEvents chuyển lịch đặt của một phòng thành các ca dọn.
//
// Quy tắc hạn dọn, theo đúng vận hành đã chốt:
//   - Có khách nhận phòng ngay trong ngày → hạn là giờ khách đó vào, nhưng không
//     bao giờ ngắn hơn `clean_time` (đệm dọn dẹp cấu hình ở listing, tối thiểu 1h).
//     Ép hạn sát hơn đệm là đặt ra một mốc không ai làm kịp.
//   - Không có khách kế tiếp → hạn là giờ nhận phòng chuẩn của căn.
func hkTurnsFromEvents(room HKRoom, events []hkICalEvent, cleanTimeHours int, loc *time.Location) []hkTurn {
	if cleanTimeHours <= 0 {
		cleanTimeHours = 1
	}
	// Mọi ngày có khách nhận phòng, kể cả lượt chủ nhà khoá: phòng bị chiếm thì
	// vẫn phải xong trước giờ đó.
	starts := map[string]bool{}
	for _, e := range events {
		if !e.Start.IsZero() {
			starts[e.Start.Format("2006-01-02")] = true
		}
	}

	var out []hkTurn
	for _, e := range events {
		if e.Blocked || e.End.IsZero() {
			continue
		}
		day := e.End.Format("2006-01-02")
		dayStart := time.Date(e.End.Year(), e.End.Month(), e.End.Day(), 0, 0, 0, 0, loc)

		checkout := dayStart.Add(time.Duration(room.CheckoutHr) * time.Hour)
		hasNext := starts[day]

		deadline := dayStart.Add(time.Duration(room.CheckinHr) * time.Hour)
		if hasNext {
			minDone := checkout.Add(time.Duration(cleanTimeHours) * time.Hour)
			if deadline.Before(minDone) {
				deadline = minDone
			}
		}

		out = append(out, hkTurn{
			RoomID:     room.ID,
			ListingID:  room.ListingID,
			UID:        e.UID,
			Day:        day,
			CheckoutAt: checkout,
			DeadlineAt: deadline,
			HasNext:    hasNext,
		})
	}
	return out
}
