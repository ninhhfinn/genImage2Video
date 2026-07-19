package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// withIntroDir trỏ assetsDir về TempDir (qua CWD) để test không đụng assets thật.
// assetsDir() thử "assets" CWD-relative trước → chdir vào tmp có sẵn assets/intro.
func withIntroDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "assets", "intro")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	// reset trạng thái pick giữa các test
	introPickMu.Lock()
	lastIntroPick = ""
	introPickMu.Unlock()
	return dir
}

func touchClip(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSafeIntroName(t *testing.T) {
	cases := map[string]string{
		"Cảnh Đi Đường.MOV":  "Cnh_i_ng.mp4", // ký tự có dấu bị loại, khoảng trắng → _
		"clip 1.mp4":         "clip_1.mp4",
		"../../etc/passwd":   "passwd.mp4",
		"street-view_2.webm": "street-view_2.mp4",
	}
	for in, want := range cases {
		if got := safeIntroName(in); got != want {
			t.Errorf("safeIntroName(%q)=%q, muốn %q", in, got, want)
		}
	}
	// Tên rỗng sau sanitize → fallback hash, vẫn .mp4
	if got := safeIntroName("我们.mov"); !strings.HasPrefix(got, "intro_") || !strings.HasSuffix(got, ".mp4") {
		t.Errorf("tên toàn ký tự loại phải fallback intro_*.mp4, được %q", got)
	}
}

func TestResolveIntroPathTraversal(t *testing.T) {
	dir := withIntroDir(t)
	touchClip(t, dir, "a.mp4")
	if resolveIntroPath("a.mp4") == "" {
		t.Error("file có thật phải resolve được")
	}
	for _, bad := range []string{"../a.mp4", "sub/a.mp4", "..", ".", "", "khongco.mp4"} {
		if p := resolveIntroPath(bad); p != "" {
			t.Errorf("resolveIntroPath(%q) phải trả \"\" (traversal/không tồn tại), được %q", bad, p)
		}
	}
}

func TestPickIntroClip(t *testing.T) {
	dir := withIntroDir(t)

	// Rỗng → ""
	if p := pickIntroClip(); p != "" {
		t.Errorf("thư viện rỗng phải trả \"\", được %q", p)
	}

	// 1 clip → luôn chính nó
	touchClip(t, dir, "only.mp4")
	for i := 0; i < 5; i++ {
		if filepath.Base(pickIntroClip()) != "only.mp4" {
			t.Fatal("1 clip phải luôn bốc chính nó")
		}
	}

	// ≥2 clip → không lặp clip vừa dùng ở lượt kế
	touchClip(t, dir, "b.mp4")
	touchClip(t, dir, "c.mp4")
	prev := filepath.Base(pickIntroClip())
	for i := 0; i < 30; i++ {
		cur := filepath.Base(pickIntroClip())
		if cur == prev {
			t.Fatalf("bốc lặp clip vừa dùng: %q hai lần liên tiếp", cur)
		}
		prev = cur
	}
}

func TestPickIntroClipIgnoresNonMp4(t *testing.T) {
	dir := withIntroDir(t)
	touchClip(t, dir, "keep.mp4")
	touchClip(t, dir, "skip.mov")
	touchClip(t, dir, "skip.txt")
	for i := 0; i < 5; i++ {
		if filepath.Base(pickIntroClip()) != "keep.mp4" {
			t.Fatal("chỉ được bốc .mp4 đã chuẩn hoá, bỏ qua .mov/.txt")
		}
	}
}

// TestNormalizeIntroClip: gated ffmpeg — sinh clip testsrc rồi chuẩn hoá, kiểm
// tra output 1080×1920/30fps/không audio.
func TestNormalizeIntroClip(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("cần ffmpeg")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("cần ffprobe")
	}
	dir := withIntroDir(t)
	src := filepath.Join(t.TempDir(), "src.mp4")
	// Clip nguồn 4:3 720x480 có audio để chắc chuẩn hoá crop + strip audio.
	if out, err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=720x480:rate=25:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:v", "libx264", "-c:a", "aac", "-pix_fmt", "yuv420p", src,
	).CombinedOutput(); err != nil {
		t.Fatalf("tạo clip nguồn: %v\n%s", err, out)
	}
	dst := filepath.Join(dir, "out.mp4")
	if err := normalizeIntroClip(src, dst); err != nil {
		t.Fatalf("normalizeIntroClip: %v", err)
	}
	// ffprobe: video 1080x1920, no audio stream.
	probe := func(args ...string) string {
		out, _ := exec.Command("ffprobe", append([]string{"-v", "error"}, args...)...).Output()
		return strings.TrimSpace(string(out))
	}
	wh := probe("-select_streams", "v:0", "-show_entries", "stream=width,height",
		"-of", "csv=p=0:s=x", dst)
	if wh != "1080x1920" {
		t.Errorf("kích thước = %q, muốn 1080x1920", wh)
	}
	acodec := probe("-select_streams", "a", "-show_entries", "stream=codec_type",
		"-of", "default=nokey=1:noprint_wrappers=1", dst)
	if acodec != "" {
		t.Errorf("output phải KHÔNG có audio, ffprobe thấy %q", acodec)
	}
}

func TestUploadAndDeleteIntroHandler(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("cần ffmpeg để chuẩn hoá upload")
	}
	dir := withIntroDir(t)
	src := filepath.Join(t.TempDir(), "src.mp4")
	if out, err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=640x640:rate=25:duration=1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", src,
	).CombinedOutput(); err != nil {
		t.Fatalf("tạo clip nguồn: %v\n%s", err, out)
	}
	raw, _ := os.ReadFile(src)

	// multipart body field "intro"
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("intro", "my clip.mp4")
	fw.Write(raw)
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/upload-intro", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	uploadIntroHandler()(rec, req)
	if rec.Code != 200 {
		t.Fatalf("upload code=%d body=%s", rec.Code, rec.Body.String())
	}
	var up struct {
		OK    bool              `json:"ok"`
		Intro map[string]string `json:"intro"`
	}
	json.Unmarshal(rec.Body.Bytes(), &up)
	if !up.OK || up.Intro["file"] != "my_clip.mp4" {
		t.Fatalf("upload trả bất thường: %s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "my_clip.mp4")); err != nil {
		t.Fatalf("file chuẩn hoá không thấy trong introDir: %v", err)
	}

	// list thấy 1 clip
	lrec := httptest.NewRecorder()
	listIntrosHandler()(lrec, httptest.NewRequest(http.MethodGet, "/api/intros", nil))
	if !strings.Contains(lrec.Body.String(), "my_clip.mp4") {
		t.Errorf("list thiếu clip vừa upload: %s", lrec.Body.String())
	}

	// delete
	drec := httptest.NewRecorder()
	dreq := httptest.NewRequest(http.MethodPost, "/api/delete-intro",
		strings.NewReader(`{"intro":"my_clip.mp4"}`))
	deleteIntroHandler()(drec, dreq)
	if _, err := os.Stat(filepath.Join(dir, "my_clip.mp4")); !os.IsNotExist(err) {
		t.Errorf("delete-intro không xoá file")
	}

	// delete traversal không xoá bậy
	touchClip(t, dir, "safe.mp4")
	drec2 := httptest.NewRecorder()
	deleteIntroHandler()(drec2, httptest.NewRequest(http.MethodPost, "/api/delete-intro",
		strings.NewReader(`{"intro":"../safe.mp4"}`)))
	if _, err := os.Stat(filepath.Join(dir, "safe.mp4")); err != nil {
		t.Errorf("delete-intro với path traversal KHÔNG được xoá file khác")
	}
}
