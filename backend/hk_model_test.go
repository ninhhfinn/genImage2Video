package main

import "testing"

func hkTestTemplate() *HKTemplate {
	return &HKTemplate{
		ID: "tpl",
		Groups: []HKGroup{
			{ID: "g1", Title: "Phòng ngủ", Items: []HKItem{
				{ID: "a", Title: "Thay ga", RequirePhoto: true},
				{ID: "b", Title: "Hút bụi", RequirePhoto: true, MinPhotos: 2},
			}},
			{ID: "g2", Title: "Kiểm tra cuối", Items: []HKItem{
				{ID: "c", Title: "Báo hỏng hóc", RequirePhoto: false},
			}},
		},
	}
}

func hkPhotos(n int) []HKPhoto {
	out := make([]HKPhoto, n)
	for i := range out {
		out[i] = HKPhoto{URL: "/api/hk/photo/x.jpg"}
	}
	return out
}

func hkSessionWith(state map[string]HKItemState) *HKSession {
	return &HKSession{ItemsState: state, BaseFee: 100000}
}

// ─── Tiến độ checklist ────────────────────────────────────────────────────

func TestProgressEmptySession(t *testing.T) {
	p := hkSessionProgress(hkSessionWith(map[string]HKItemState{}), hkTestTemplate())
	if p.TotalRequired != 2 || p.DoneRequired != 0 || p.Percent != 0 || p.Complete {
		t.Fatalf("ca rỗng phải là 0/2, chưa xong: %+v", p)
	}
}

func TestProgressMinPhotosRespected(t *testing.T) {
	one := hkSessionWith(map[string]HKItemState{"b": {Photos: hkPhotos(1)}})
	if hkItemDone(one, HKItem{ID: "b", RequirePhoto: true, MinPhotos: 2}) {
		t.Fatal("1 ảnh mà mục đòi 2 thì chưa được tính là xong")
	}
	two := hkSessionWith(map[string]HKItemState{"b": {Photos: hkPhotos(2)}})
	if !hkItemDone(two, HKItem{ID: "b", RequirePhoto: true, MinPhotos: 2}) {
		t.Fatal("đủ 2 ảnh phải tính là xong")
	}
}

// Mục không yêu cầu ảnh không được chặn nộp — chặn tiền vì một cái tick không có
// bằng chứng thì chỉ dạy người ta tick bừa.
func TestProgressOptionalItemDoesNotBlock(t *testing.T) {
	s := hkSessionWith(map[string]HKItemState{
		"a": {Photos: hkPhotos(1)},
		"b": {Photos: hkPhotos(2)},
	})
	p := hkSessionProgress(s, hkTestTemplate())
	if !p.Complete {
		t.Fatalf("đủ ảnh bắt buộc là phải xong dù mục tuỳ chọn chưa tick: %+v", p)
	}
}

func TestProgressPercentUsesRequiredOnly(t *testing.T) {
	s := hkSessionWith(map[string]HKItemState{"a": {Photos: hkPhotos(1)}})
	if got := hkSessionProgress(s, hkTestTemplate()).Percent; got != 50 {
		t.Fatalf("phần trăm phải tính trên mục bắt buộc, muốn 50 được %d", got)
	}
}

func TestProgressIgnoresEmptyPhotoURLs(t *testing.T) {
	s := hkSessionWith(map[string]HKItemState{"a": {Photos: []HKPhoto{{URL: ""}, {URL: "   "}}}})
	if hkSessionProgress(s, hkTestTemplate()).DoneRequired != 0 {
		t.Fatal("ảnh có URL rỗng không được tính là đã chụp")
	}
}

func TestProgressListsMissingTitles(t *testing.T) {
	s := hkSessionWith(map[string]HKItemState{"a": {Photos: hkPhotos(1)}})
	p := hkSessionProgress(s, hkTestTemplate())
	if len(p.Missing) != 1 || p.Missing[0] != "Hút bụi" {
		t.Fatalf("phải nêu đúng tên mục còn thiếu: %+v", p.Missing)
	}
}

// ─── Suy trạng thái ───────────────────────────────────────────────────────

func TestDeriveStatus(t *testing.T) {
	tpl := hkTestTemplate()
	cases := []struct {
		name string
		sess *HKSession
		want string
	}{
		{"chưa động gì", hkSessionWith(map[string]HKItemState{}), HKSessionTodo},
		{"có ảnh chưa đủ", hkSessionWith(map[string]HKItemState{"a": {Photos: hkPhotos(1)}}), HKSessionInProgress},
		{"đủ ảnh", hkSessionWith(map[string]HKItemState{
			"a": {Photos: hkPhotos(1)}, "b": {Photos: hkPhotos(2)},
		}), HKSessionSubmitted},
	}
	for _, c := range cases {
		if got := hkDeriveStatus(c.sess, tpl); got != c.want {
			t.Errorf("%s: muốn %s được %s", c.name, c.want, got)
		}
	}
}

// Cờ status cũ trong DB không được thắng dữ liệu thật: ca đủ ảnh mà kẹt ở
// "đang dọn" là giữ tiền của cô lại vì một lần ghi bị rớt.
func TestDeriveStatusOverridesStaleFlag(t *testing.T) {
	s := hkSessionWith(map[string]HKItemState{"a": {Photos: hkPhotos(1)}, "b": {Photos: hkPhotos(2)}})
	s.Status = HKSessionTodo
	if got := hkDeriveStatus(s, hkTestTemplate()); got != HKSessionSubmitted {
		t.Fatalf("muốn submitted được %s", got)
	}
}

func TestDeriveStatusKeepsAdminDecision(t *testing.T) {
	approved := hkSessionWith(map[string]HKItemState{})
	approved.Status = HKSessionApproved
	if got := hkDeriveStatus(approved, hkTestTemplate()); got != HKSessionApproved {
		t.Fatalf("quyết định duyệt của quản lý phải giữ nguyên, được %s", got)
	}
	rejected := hkSessionWith(map[string]HKItemState{"a": {Photos: hkPhotos(1)}, "b": {Photos: hkPhotos(2)}})
	rejected.Status = HKSessionRejected
	if got := hkDeriveStatus(rejected, hkTestTemplate()); got != HKSessionRejected {
		t.Fatalf("từ chối phải giữ nguyên dù đủ ảnh, được %s", got)
	}
}

// Sửa mẫu checklist KHÔNG được đổi điều kiện của ca đang dở.
func TestTemplateSnapshotWins(t *testing.T) {
	snapshot := &HKTemplate{ID: "tpl", Groups: []HKGroup{
		{ID: "g1", Items: []HKItem{{ID: "a", Title: "Thay ga", RequirePhoto: true}}},
	}}
	s := hkSessionWith(map[string]HKItemState{"a": {Photos: hkPhotos(1)}})
	s.TemplateSnapshot = snapshot

	live := hkTestTemplate() // mẫu mới thêm mục "b" đòi 2 ảnh
	if got := hkDeriveStatus(s, hkTemplateFor(s, live)); got != HKSessionSubmitted {
		t.Fatalf("ca đang dở phải chấm theo mẫu lúc tạo, được %s", got)
	}
}

// ─── Tính công ────────────────────────────────────────────────────────────

func TestPayApprovedAndSubmitted(t *testing.T) {
	approved := &HKSession{Status: HKSessionApproved, BaseFee: 100000}
	if p := hkComputePay(approved); p.Total != 100000 || !p.Confirmed {
		t.Fatalf("ca đã duyệt: %+v", p)
	}
	// Ca chờ đối soát VẪN ra tiền — cô làm xong là thấy tiền ngay.
	submitted := &HKSession{Status: HKSessionSubmitted, BaseFee: 100000}
	p := hkComputePay(submitted)
	if p.Total != 100000 || p.Confirmed || !p.Payable {
		t.Fatalf("ca chờ đối soát phải ra tiền nhưng chưa chốt: %+v", p)
	}
}

func TestPayNotPayableStates(t *testing.T) {
	for _, st := range []string{HKSessionTodo, HKSessionInProgress, HKSessionRejected} {
		if p := hkComputePay(&HKSession{Status: st, BaseFee: 100000}); p.Total != 0 {
			t.Errorf("trạng thái %s không được ra tiền, được %d", st, p.Total)
		}
	}
}

// Phụ cấp chờ duyệt không cộng vào tổng, kể cả cột tạm tính: hiện như tiền đã có
// rồi sau đó quản lý cắt đi mới là thứ gây cãi nhau.
func TestPayPendingAllowanceExcluded(t *testing.T) {
	s := &HKSession{Status: HKSessionApproved, BaseFee: 100000, Allowances: []HKAllowance{
		{Amount: 30000, Status: HKAllowancePending},
	}}
	p := hkComputePay(s)
	if p.Total != 100000 || p.AllowancePending != 30000 || p.Allowance != 0 {
		t.Fatalf("%+v", p)
	}
}

func TestPayApprovedAllowanceAdded(t *testing.T) {
	s := &HKSession{Status: HKSessionApproved, BaseFee: 100000, Allowances: []HKAllowance{
		{Amount: 30000, Status: HKAllowanceApproved},
	}}
	if p := hkComputePay(s); p.Total != 130000 {
		t.Fatalf("muốn 130000 được %d", p.Total)
	}
}

func TestPayRejectedAllowanceIgnored(t *testing.T) {
	s := &HKSession{Status: HKSessionApproved, BaseFee: 100000, Allowances: []HKAllowance{
		{Amount: 30000, Status: HKAllowanceRejected},
	}}
	p := hkComputePay(s)
	if p.Total != 100000 || p.AllowancePending != 0 {
		t.Fatalf("%+v", p)
	}
}

// Công không bao giờ âm — trừ quá tay là lỗi nhập liệu, không phải chính sách.
func TestPayDeductionCappedAtBase(t *testing.T) {
	s := &HKSession{Status: HKSessionApproved, BaseFee: 100000, Deduction: 500000}
	p := hkComputePay(s)
	if p.Deduction != 100000 || p.Total != 0 {
		t.Fatalf("%+v", p)
	}
}

func TestPayNegativeDeductionIgnored(t *testing.T) {
	s := &HKSession{Status: HKSessionApproved, BaseFee: 100000, Deduction: -50000}
	if p := hkComputePay(s); p.Total != 100000 {
		t.Fatalf("trừ âm không được thành thưởng, được %d", p.Total)
	}
}

func TestPayPendingAllowanceCountedEvenWhenUnpayable(t *testing.T) {
	s := &HKSession{Status: HKSessionInProgress, BaseFee: 100000, Allowances: []HKAllowance{
		{Amount: 20000, Status: HKAllowancePending},
	}}
	p := hkComputePay(s)
	if p.Total != 0 || p.AllowancePending != 20000 {
		t.Fatalf("%+v", p)
	}
}

// ─── Bảng công ────────────────────────────────────────────────────────────

func TestTimesheetSeparatesConfirmedAndProvisional(t *testing.T) {
	users := map[string]HKUser{"u1": {ID: "u1", Name: "Lan", Phone: "091"}}
	rows := hkBuildTimesheet([]HKSession{
		{StaffID: "u1", Status: HKSessionApproved, BaseFee: 100000},
		{StaffID: "u1", Status: HKSessionSubmitted, BaseFee: 80000},
		{StaffID: "u1", Status: HKSessionRejected, BaseFee: 140000},
	}, users)
	if len(rows) != 1 {
		t.Fatalf("muốn 1 dòng được %d", len(rows))
	}
	r := rows[0]
	if r.ConfirmedTotal != 100000 || r.ProvisionalTotal != 80000 || r.Total != 180000 {
		t.Fatalf("%+v", r)
	}
	if r.Rooms != 2 || r.Rejected != 1 {
		t.Fatalf("đếm phòng sai: %+v", r)
	}
}

func TestTimesheetSortedByTotalDesc(t *testing.T) {
	users := map[string]HKUser{
		"u1": {ID: "u1", Name: "Lan"},
		"u2": {ID: "u2", Name: "Hoa"},
	}
	rows := hkBuildTimesheet([]HKSession{
		{StaffID: "u1", Status: HKSessionApproved, BaseFee: 80000},
		{StaffID: "u2", Status: HKSessionApproved, BaseFee: 140000},
		{StaffID: "u2", Status: HKSessionApproved, BaseFee: 100000},
	}, users)
	if rows[0].Name != "Hoa" || rows[0].Total != 240000 {
		t.Fatalf("phải sắp theo tổng giảm dần: %+v", rows)
	}
}

func TestTimesheetSkipsUnassigned(t *testing.T) {
	rows := hkBuildTimesheet([]HKSession{{StaffID: "", Status: HKSessionApproved, BaseFee: 100000}}, nil)
	if len(rows) != 0 {
		t.Fatalf("ca chưa xếp người không tạo dòng: %+v", rows)
	}
}

// Ca trỏ tới tài khoản đã xoá vẫn phải hiện — nuốt mất nó là nuốt mất tiền của
// một người đã làm việc thật.
func TestTimesheetKeepsOrphanSession(t *testing.T) {
	rows := hkBuildTimesheet([]HKSession{{StaffID: "ghost", Status: HKSessionApproved, BaseFee: 100000}}, nil)
	if len(rows) != 1 || rows[0].Name != "Không rõ" {
		t.Fatalf("%+v", rows)
	}
}

// ─── Chuẩn hoá số điện thoại ──────────────────────────────────────────────

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"0912345601":     "0912345601",
		"0912 345 601":   "0912345601",
		"+84912345601":   "0912345601",
		"84912345601":    "0912345601",
		"(091) 234-5601": "0912345601",
	}
	for in, want := range cases {
		if got := hkNormalizePhone(in); got != want {
			t.Errorf("%q → muốn %q được %q", in, want, got)
		}
	}
}

// ─── Suy loại phòng ───────────────────────────────────────────────────────

func TestRoomTypeFromBedrooms(t *testing.T) {
	cases := map[int]string{0: "studio", 1: "one_bedroom", 2: "two_bedroom", 3: "duplex", 5: "duplex"}
	for in, want := range cases {
		if got := hkRoomTypeFromBedrooms(in); got != want {
			t.Errorf("%d phòng ngủ → muốn %s được %s", in, want, got)
		}
	}
	if hkDefaultBaseFee("two_bedroom") != 140000 {
		t.Fatal("đơn giá 2PN sai")
	}
}
