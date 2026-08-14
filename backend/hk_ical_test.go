package main

import (
	"testing"
	"time"
)

// Mẫu lấy từ feed thật của api.dayladau.com — giữ nguyên định dạng, kể cả dòng
// "Blocked by dayladau" là lượt chủ nhà tự khoá.
const hkSampleICal = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//arran4//Golang ICS Library
BEGIN:VEVENT
UID:rez-aaa@dayladau.com
DTSTART;VALUE=DATE:20260813
DTEND;VALUE=DATE:20260814
SUMMARY:Reserved for dayladau
END:VEVENT
BEGIN:VEVENT
UID:rez-bbb@dayladau.com
DTSTART;VALUE=DATE:20260814
DTEND;VALUE=DATE:20260816
SUMMARY:Reserved for dayladau
END:VEVENT
BEGIN:VEVENT
UID:blk-ccc@dayladau.com
DTSTART;VALUE=DATE:20260820
DTEND;VALUE=DATE:20260822
SUMMARY:Blocked by dayladau
END:VEVENT
END:VCALENDAR`

func hkTestRoom() HKRoom {
	return HKRoom{
		ID: "r1", ListingID: "ls1", CheckinHr: 14, CheckoutHr: 11, CleanTime: 1,
	}
}

func TestParseICal(t *testing.T) {
	evs, err := hkParseICal(hkSampleICal, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 3 {
		t.Fatalf("muốn 3 sự kiện được %d", len(evs))
	}
	if evs[0].UID != "rez-aaa@dayladau.com" {
		t.Fatalf("UID sai: %q", evs[0].UID)
	}
	if evs[0].End.Format("2006-01-02") != "2026-08-14" {
		t.Fatalf("ngày trả phòng sai: %s", evs[0].End)
	}
	if !evs[2].Blocked {
		t.Fatal("lượt 'Blocked by dayladau' phải bị đánh dấu là khoá lịch")
	}
	if evs[0].Blocked || evs[1].Blocked {
		t.Fatal("lượt khách thật không được coi là khoá lịch")
	}
}

// Lượt chủ nhà tự khoá KHÔNG sinh ca — không có khách thì không có gì để dọn,
// tạo ca ở đó là giao việc không tồn tại cho người thật.
func TestBlockedDatesMakeNoSession(t *testing.T) {
	evs, _ := hkParseICal(hkSampleICal, time.Local)
	turns := hkTurnsFromEvents(hkTestRoom(), evs, 1, time.Local)
	if len(turns) != 2 {
		t.Fatalf("muốn 2 ca (bỏ lượt khoá) được %d", len(turns))
	}
	for _, tr := range turns {
		if tr.Day == "2026-08-22" {
			t.Fatal("ngày kết thúc lượt khoá không được thành ca dọn")
		}
	}
}

// Khách sau nhận phòng ngay trong ngày → hạn dọn là giờ khách đó vào.
func TestDeadlineWhenNextGuestSameDay(t *testing.T) {
	evs, _ := hkParseICal(hkSampleICal, time.Local)
	turns := hkTurnsFromEvents(hkTestRoom(), evs, 1, time.Local)

	var turn14 hkTurn
	for _, tr := range turns {
		if tr.Day == "2026-08-14" {
			turn14 = tr
		}
	}
	if turn14.Day == "" {
		t.Fatal("thiếu ca ngày 14/08")
	}
	if !turn14.HasNext {
		t.Fatal("ngày 14/08 có khách nhận phòng nên phải gắn cờ HasNext")
	}
	if h := turn14.DeadlineAt.Hour(); h != 14 {
		t.Fatalf("hạn phải là 14:00 (giờ khách sau vào), được %d:00", h)
	}
}

// Đệm dọn dẹp là sàn cứng: khách sau vào lúc 11h30 mà đệm 1h thì hạn phải là
// 12:00, không phải 11:30 — ép sát hơn đệm là đặt ra mốc không ai làm kịp.
func TestDeadlineNeverShorterThanCleanBuffer(t *testing.T) {
	room := hkTestRoom()
	room.CheckinHr = 11 // khách sau vào ngay 11h, cùng giờ trả phòng
	evs, _ := hkParseICal(hkSampleICal, time.Local)
	turns := hkTurnsFromEvents(room, evs, 2, time.Local) // đệm 2 tiếng

	for _, tr := range turns {
		if tr.Day != "2026-08-14" {
			continue
		}
		if h := tr.DeadlineAt.Hour(); h != 13 {
			t.Fatalf("trả 11:00 + đệm 2h ⇒ hạn 13:00, được %d:00", h)
		}
		return
	}
	t.Fatal("thiếu ca ngày 14/08")
}

// Không có khách kế tiếp → hạn mềm là giờ nhận phòng chuẩn của căn.
func TestDeadlineWhenNoNextGuest(t *testing.T) {
	evs, _ := hkParseICal(hkSampleICal, time.Local)
	turns := hkTurnsFromEvents(hkTestRoom(), evs, 1, time.Local)
	for _, tr := range turns {
		if tr.Day != "2026-08-16" {
			continue
		}
		if tr.HasNext {
			t.Fatal("ngày 16/08 không có khách nhận phòng")
		}
		if h := tr.DeadlineAt.Hour(); h != 14 {
			t.Fatalf("hạn mềm phải là 14:00, được %d:00", h)
		}
		return
	}
	t.Fatal("thiếu ca ngày 16/08")
}

func TestParseICalHandlesEmptyAndGarbage(t *testing.T) {
	for _, in := range []string{"", "không phải iCal", "BEGIN:VCALENDAR\nEND:VCALENDAR"} {
		evs, err := hkParseICal(in, time.Local)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if len(evs) != 0 {
			t.Fatalf("%q: muốn 0 sự kiện được %d", in, len(evs))
		}
	}
}

// Sự kiện thiếu DTEND bị bỏ qua thay vì tạo ca với ngày rỗng.
func TestEventWithoutEndIgnored(t *testing.T) {
	ics := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:x\nDTSTART;VALUE=DATE:20260813\nSUMMARY:Reserved\nEND:VEVENT\nEND:VCALENDAR"
	evs, _ := hkParseICal(ics, time.Local)
	if len(evs) != 0 {
		t.Fatalf("muốn 0 được %d", len(evs))
	}
}

// ─── Chỉ số hiệu suất ─────────────────────────────────────────────────────

func TestSessionMinutes(t *testing.T) {
	base := int64(1_700_000_000_000)
	s := &HKSession{StartedAt: base, SubmittedAt: base + 45*60*1000}
	if m := hkSessionMinutes(s); m != 45 {
		t.Fatalf("muốn 45 phút được %d", m)
	}
	// Thiếu mốc → 0, không đoán bừa.
	if m := hkSessionMinutes(&HKSession{SubmittedAt: base}); m != 0 {
		t.Fatalf("thiếu mốc bắt đầu phải trả 0, được %d", m)
	}
	// Quá 8 tiếng gần như chắc chắn là quên bấm bắt đầu từ hôm trước — loại khỏi
	// trung bình để một bản ghi hỏng không kéo lệch cả báo cáo.
	if m := hkSessionMinutes(&HKSession{StartedAt: base, SubmittedAt: base + 10*3600*1000}); m != 0 {
		t.Fatalf("ca 10 tiếng phải bị loại, được %d", m)
	}
}

func TestBuildPerf(t *testing.T) {
	base := int64(1_700_000_000_000)
	users := map[string]HKUser{"u1": {ID: "u1", Name: "Lan", Phone: "091"}}
	sessions := []HKSession{
		{StaffID: "u1", RoomID: "rA", Status: HKSessionApproved, StartedAt: base, SubmittedAt: base + 30*60*1000},
		{StaffID: "u1", RoomID: "rA", Status: HKSessionSubmitted, StartedAt: base, SubmittedAt: base + 50*60*1000},
		{StaffID: "u1", RoomID: "rB", Status: HKSessionRejected},
		{StaffID: "u1", RoomID: "rC", Status: HKSessionInProgress}, // bỏ dở, không tính
	}
	rows := hkBuildPerf(sessions, users, nil)
	if len(rows) != 1 {
		t.Fatalf("muốn 1 dòng được %d", len(rows))
	}
	r := rows[0]
	if r.Sessions != 2 {
		t.Fatalf("chỉ ca đủ ảnh mới tính, muốn 2 được %d", r.Sessions)
	}
	if r.Rooms != 1 {
		t.Fatalf("hai ca cùng phòng rA ⇒ 1 phòng, được %d", r.Rooms)
	}
	if r.AvgMinute != 40 {
		t.Fatalf("trung bình (30+50)/2 = 40, được %d", r.AvgMinute)
	}
	if r.Approved != 1 || r.Pending != 1 || r.Rejected != 1 {
		t.Fatalf("đếm trạng thái sai: %+v", r)
	}
}

func TestPerfCountsLate(t *testing.T) {
	base := int64(1_700_000_000_000)
	users := map[string]HKUser{"u1": {ID: "u1", Name: "Lan"}}
	rows := hkBuildPerf([]HKSession{
		{StaffID: "u1", RoomID: "rA", Status: HKSessionApproved, DeadlineAt: base, SubmittedAt: base + 60_000},
		{StaffID: "u1", RoomID: "rB", Status: HKSessionApproved, DeadlineAt: base, SubmittedAt: base - 60_000},
	}, users, nil)
	if rows[0].Late != 1 {
		t.Fatalf("muốn 1 ca trễ được %d", rows[0].Late)
	}
}

// ─── Lọc review liên quan dọn dẹp ─────────────────────────────────────────

func TestReviewAboutCleaning(t *testing.T) {
	cases := []struct {
		name string
		rv   HKReview
		want bool
	}{
		{"điểm sạch sẽ thấp", HKReview{Cleanliness: 2, Comment: "ok"}, true},
		{"chê không thay ga", HKReview{Cleanliness: 5, Comment: "Home ko thay ga đến giờ"}, true},
		{"nhắc mùi", HKReview{Cleanliness: 5, Comment: "Phòng có mùi lạ"}, true},
		{"khen chung chung", HKReview{Cleanliness: 5, Comment: "Chủ nhà thân thiện"}, false},
		{"không nội dung", HKReview{Cleanliness: 5}, false},
	}
	for _, c := range cases {
		if got := hkIsAboutCleaning(c.rv); got != c.want {
			t.Errorf("%s: muốn %v được %v", c.name, c.want, got)
		}
	}
}

func TestSummarizeReviews(t *testing.T) {
	st := hkSummarizeReviews([]HKReview{
		{Overall: 5, Cleanliness: 5, Comment: "Sạch sẽ"},
		{Overall: 1, Cleanliness: 1, Comment: "Ko thay ga", AboutCleaning: true},
		{Overall: 5, Cleanliness: 4},
	})
	if st.Total != 3 || st.FiveStar != 2 || st.LowClean != 1 {
		t.Fatalf("%+v", st)
	}
	if st.AvgCleanliness < 3.3 || st.AvgCleanliness > 3.34 {
		t.Fatalf("trung bình sạch sẽ (5+1+4)/3 ≈ 3.33, được %.2f", st.AvgCleanliness)
	}
	if len(st.NeedAttention) != 1 {
		t.Fatalf("muốn 1 review cần chú ý được %d", len(st.NeedAttention))
	}
}

func TestParseTimeMs(t *testing.T) {
	if got := hkParseTimeMs([]byte("1786000000")); got != 1786000000000 {
		t.Fatalf("giây phải đổi ra mili-giây, được %d", got)
	}
	if got := hkParseTimeMs([]byte("1786000000000")); got != 1786000000000 {
		t.Fatalf("mili-giây giữ nguyên, được %d", got)
	}
	if got := hkParseTimeMs([]byte("\"rác\"")); got != 0 {
		t.Fatalf("giá trị hỏng phải trả 0, được %d", got)
	}
}
