package main

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ─── Thư viện clip "cảnh đi đường" (intro) cho mode narrated ────────────────
//
// Thay cho street.mp4 cố định: người dùng upload nhiều clip qua web (hoặc nhập
// từ link Google Drive), mỗi clip được chuẩn hoá về 1080×1920/30fps/không tiếng.
// Mỗi lần render, buildNarrated bốc NGẪU NHIÊN 1 clip (tránh lặp clip vừa dùng).
// Thư viện rỗng → introAssetPath() trả "" → tự bỏ intro (giữ graceful-skip cũ).

const maxIntroBytes = 300 << 20 // 300MB — headroom cho .MOV iPhone thô

// introUploadExts: đuôi nhận khi UPLOAD (thư viện chỉ LƯU .mp4 đã chuẩn hoá).
var introUploadExts = map[string]bool{".mp4": true, ".mov": true, ".m4v": true, ".webm": true}

func introDir() string { return filepath.Join(assetsDir(), "intro") }

// resolveIntroPath: map tên (từ request) → path thật trong introDir, "" nếu
// thiếu/không tồn tại. Chỉ nhận base name (chống path traversal). Clone
// resolveMusicPath (music.go).
func resolveIntroPath(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	base := filepath.Base(name)
	if base == "." || base == ".." || base != name {
		return ""
	}
	p := filepath.Join(introDir(), base)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

// safeIntroName: sanitize tên gốc → tên file an toàn, LUÔN đuôi .mp4 (clip đã
// chuẩn hoá). Clone safeMusicName nhưng ép .mp4.
func safeIntroName(original string) string {
	stem := strings.TrimSuffix(filepath.Base(original), filepath.Ext(original))
	var b strings.Builder
	for _, r := range stem {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		}
	}
	name := b.String()
	if name == "" {
		sum := sha1.Sum([]byte(original))
		name = fmt.Sprintf("intro_%x", sum[:4])
	}
	return name + ".mp4"
}

// uniqueIntroName: nếu tên đã tồn tại trong introDir thì thêm hậu tố _1, _2…
func uniqueIntroName(name string) string {
	base := strings.TrimSuffix(name, ".mp4")
	candidate := name
	for i := 1; ; i++ {
		if _, err := os.Stat(filepath.Join(introDir(), candidate)); err != nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s_%d.mp4", base, i)
	}
}

// listIntroClips: danh sách .mp4 trong introDir (đã sort).
func listIntroClips() []string {
	out := []string{}
	entries, _ := os.ReadDir(introDir())
	for _, e := range entries {
		if !e.IsDir() && strings.ToLower(filepath.Ext(e.Name())) == ".mp4" {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

var (
	introPickMu   sync.Mutex
	lastIntroPick string
)

// pickIntroClip: bốc ngẫu nhiên 1 clip .mp4 trong introDir. Khi có ≥2 clip,
// tránh lặp lại clip vừa dùng ở render trước. Thư viện rỗng → "".
func pickIntroClip() string {
	clips := listIntroClips()
	if len(clips) == 0 {
		return ""
	}
	introPickMu.Lock()
	defer introPickMu.Unlock()
	if len(clips) == 1 {
		lastIntroPick = clips[0]
		return filepath.Join(introDir(), clips[0])
	}
	// Loại clip vừa dùng để không lặp liên tiếp.
	pool := clips[:0:0]
	for _, c := range clips {
		if c != lastIntroPick {
			pool = append(pool, c)
		}
	}
	if len(pool) == 0 {
		pool = clips
	}
	pick := pool[rand.Intn(len(pool))]
	lastIntroPick = pick
	return filepath.Join(introDir(), pick)
}

// ─── Chuẩn hoá clip → 1080×1920 / 30fps / không tiếng / h264 ─────────────────

// isHDRVideo: nhận diện clip HDR (iPhone HLG/HDR10) qua color_transfer để chèn
// tonemap, tránh video ra bị bệt/xám. Không đọc được → coi là SDR (an toàn).
func isHDRVideo(src string) bool {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return false
	}
	out, err := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=color_transfer",
		"-of", "default=nokey=1:noprint_wrappers=1",
		src,
	).Output()
	if err != nil {
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(string(out)))
	return ct == "smpte2084" || ct == "arib-std-b67"
}

// normalizeIntroClip: encode src → dst 1080×1920, 30fps, bỏ audio, h264 yuv420p
// +faststart. iPhone tự xoay đúng chiều (ffmpeg đọc rotation metadata). HDR →
// chèn chuỗi tonemap hable (cần libzimg trong ffmpeg — setup-server.sh kiểm tra).
func normalizeIntroClip(src, dst string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("không thấy ffmpeg")
	}
	scale := "scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,fps=30,setsar=1,format=yuv420p"
	vf := scale
	if isHDRVideo(src) {
		tonemap := "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,"
		vf = tonemap + scale
	}
	if err := os.MkdirAll(introDir(), 0755); err != nil {
		return err
	}
	args := []string{
		"-y", "-i", src,
		"-vf", vf,
		"-an",
		"-c:v", "libx264", "-crf", "18", "-preset", "medium",
		"-movflags", "+faststart",
		dst,
	}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg lỗi: %v\n%s", err, tailBytes(out, 500))
	}
	return nil
}

// tailBytes: lấy tối đa n byte cuối (rút gọn stderr ffmpeg cho log/response).
func tailBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return "…" + string(b[len(b)-n:])
}

// ─── POST /api/upload-intro (multipart field "intro") ───────────────────────

func uploadIntroHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxIntroBytes+(1<<20))
		// ParseMultipartForm spill xuống đĩa (32MB in-memory) — KHÔNG nạp cả
		// 300MB vào RAM như music.go.
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSONErr(w, 400, "file quá lớn hoặc form hỏng (tối đa 300MB)")
			return
		}
		file, hdr, err := r.FormFile("intro")
		if err != nil {
			writeJSONErr(w, 400, "thiếu field 'intro'")
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		if !introUploadExts[ext] {
			writeJSONErr(w, 400, "chỉ nhận mp4/mov/m4v/webm")
			return
		}

		// Ghi upload ra file tạm rồi chuẩn hoá vào introDir.
		tmp, err := os.CreateTemp("", "intro_upload_*"+ext)
		if err != nil {
			writeJSONErr(w, 500, "không tạo được file tạm")
			return
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)
		n, err := io.Copy(tmp, io.LimitReader(file, maxIntroBytes+1))
		tmp.Close()
		if err != nil || n == 0 {
			writeJSONErr(w, 400, "không đọc được file")
			return
		}
		if n > maxIntroBytes {
			writeJSONErr(w, 400, "file quá lớn (tối đa 300MB)")
			return
		}

		name := uniqueIntroName(safeIntroName(hdr.Filename))
		dst := filepath.Join(introDir(), name)
		if err := normalizeIntroClip(tmpPath, dst); err != nil {
			writeJSONErr(w, 500, "không chuẩn hoá được clip: "+err.Error())
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"ok":    true,
			"intro": map[string]string{"file": name, "label": hdr.Filename},
		})
	}
}

// ─── GET /api/intros ────────────────────────────────────────────────────────

func listIntrosHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		type introItem struct {
			File  string `json:"file"`
			Label string `json:"label"`
		}
		out := []introItem{}
		for _, f := range listIntroClips() {
			out = append(out, introItem{File: f, Label: f})
		}
		json.NewEncoder(w).Encode(out)
	}
}

// ─── POST /api/delete-intro {"intro":"<name>"} ──────────────────────────────

func deleteIntroHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}
		var body struct {
			Intro string `json:"intro"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if p := resolveIntroPath(body.Intro); p != "" {
			os.Remove(p)
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}

// ─── Import từ Google Drive (gdown, chạy nền) ───────────────────────────────

type introImportState struct {
	mu       sync.Mutex
	running  bool
	total    int
	done     int
	imported []string
	errors   []string
}

var introImport = &introImportState{imported: []string{}, errors: []string{}}

func (s *introImportState) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	imp := append([]string{}, s.imported...)
	errs := append([]string{}, s.errors...)
	return map[string]any{
		"running": s.running, "total": s.total, "done": s.done,
		"imported": imp, "errors": errs,
	}
}

// gdownBin: cho phép override qua env GDOWN_BIN; mặc định "gdown" trên PATH.
func gdownBin() string {
	if b := strings.TrimSpace(os.Getenv("GDOWN_BIN")); b != "" {
		return b
	}
	return "gdown"
}

// POST /api/import-intro-drive {"url":"..."} — tải + chuẩn hoá clip từ Drive.
func importIntroDriveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}
		var body struct {
			URL string `json:"url"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		url := strings.TrimSpace(body.URL)
		if url == "" {
			writeJSONErr(w, 400, "thiếu url")
			return
		}
		if _, err := exec.LookPath(gdownBin()); err != nil {
			writeJSONErr(w, 500, "server chưa cài gdown (chạy setup-server.sh)")
			return
		}
		introImport.mu.Lock()
		if introImport.running {
			introImport.mu.Unlock()
			writeJSONErr(w, 409, "đang có lượt nhập khác chạy")
			return
		}
		introImport.running = true
		introImport.total, introImport.done = 0, 0
		introImport.imported = []string{}
		introImport.errors = []string{}
		introImport.mu.Unlock()

		go runIntroDriveImport(url)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "started": true})
	}
}

// GET /api/intro-import-status
func introImportStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(introImport.snapshot())
	}
}

// runIntroDriveImport: gdown tải về tmp (folder → --folder, file lẻ → --fuzzy)
// rồi chuẩn hoá từng video vào introDir. Cập nhật introImport theo tiến độ.
func runIntroDriveImport(url string) {
	defer func() {
		introImport.mu.Lock()
		introImport.running = false
		introImport.mu.Unlock()
	}()

	tmpDir, err := os.MkdirTemp("", "intro_drive_*")
	if err != nil {
		introImport.addError("không tạo được thư mục tạm: " + err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	var cmd *exec.Cmd
	if strings.Contains(url, "/folders/") || strings.Contains(url, "/drive/folders") {
		cmd = exec.Command(gdownBin(), "--folder", url, "-O", tmpDir)
	} else {
		cmd = exec.Command(gdownBin(), "--fuzzy", url, "-O", tmpDir)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		introImport.addError("gdown lỗi: " + err.Error() + " | " + tailBytes(out, 400))
		return
	}

	// Gom mọi file video tải về (đệ quy — --folder tạo thư mục con).
	var vids []string
	filepath.WalkDir(tmpDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if introUploadExts[strings.ToLower(filepath.Ext(p))] {
			vids = append(vids, p)
		}
		return nil
	})
	sort.Strings(vids)

	introImport.mu.Lock()
	introImport.total = len(vids)
	introImport.mu.Unlock()

	if len(vids) == 0 {
		introImport.addError("không tìm thấy video (.mp4/.mov/.m4v/.webm) trong link Drive")
		return
	}

	for _, src := range vids {
		name := uniqueIntroName(safeIntroName(filepath.Base(src)))
		dst := filepath.Join(introDir(), name)
		if err := normalizeIntroClip(src, dst); err != nil {
			introImport.addError(filepath.Base(src) + ": " + err.Error())
		} else {
			introImport.mu.Lock()
			introImport.imported = append(introImport.imported, name)
			introImport.mu.Unlock()
		}
		introImport.mu.Lock()
		introImport.done++
		introImport.mu.Unlock()
	}
}

func (s *introImportState) addError(msg string) {
	s.mu.Lock()
	s.errors = append(s.errors, msg)
	s.mu.Unlock()
}
