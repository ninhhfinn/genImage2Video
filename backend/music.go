package main

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ─── Thư viện nhạc nền cho mode "narrated" (upload / list / delete) ──────────

const maxMusicBytes = 20 << 20 // 20MB

var musicExts = map[string]bool{".mp3": true, ".m4a": true, ".wav": true, ".aac": true}

func musicDir() string { return filepath.Join(assetsDir(), "music") }

// resolveMusicPath map tên file (từ request) → đường dẫn thật, hoặc "" nếu
// không có / không tồn tại. Chỉ nhận base name (chống path traversal).
func resolveMusicPath(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	base := filepath.Base(name)
	if base == "." || base == ".." || base != name {
		return ""
	}
	p := filepath.Join(musicDir(), base)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

func safeMusicName(original string) string {
	ext := strings.ToLower(filepath.Ext(original))
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
		name = fmt.Sprintf("music_%x", sum[:4])
	}
	return name + ext
}

// ─── POST /api/upload-music (multipart field "music") ───────────────────────

func uploadMusicHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxMusicBytes+(1<<20))
		if err := r.ParseMultipartForm(maxMusicBytes + (1 << 20)); err != nil {
			writeJSONErr(w, 400, "file quá lớn hoặc form hỏng (tối đa 20MB)")
			return
		}
		file, hdr, err := r.FormFile("music")
		if err != nil {
			writeJSONErr(w, 400, "thiếu field 'music'")
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		if !musicExts[ext] {
			writeJSONErr(w, 400, "chỉ nhận mp3/m4a/wav/aac")
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, maxMusicBytes+1))
		if err != nil || len(data) == 0 {
			writeJSONErr(w, 400, "không đọc được file")
			return
		}
		if len(data) > maxMusicBytes {
			writeJSONErr(w, 400, "file quá lớn (tối đa 20MB)")
			return
		}
		if err := os.MkdirAll(musicDir(), 0755); err != nil {
			writeJSONErr(w, 500, "không tạo được thư mục nhạc")
			return
		}
		name := safeMusicName(hdr.Filename)
		if err := os.WriteFile(filepath.Join(musicDir(), name), data, 0644); err != nil {
			writeJSONErr(w, 500, "không lưu được file nhạc")
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"ok":    true,
			"music": map[string]string{"file": name, "label": hdr.Filename},
		})
	}
}

// ─── GET /api/music ─────────────────────────────────────────────────────────

func listMusicHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		type musicItem struct {
			File  string `json:"file"`
			Label string `json:"label"`
		}
		out := []musicItem{}
		entries, _ := os.ReadDir(musicDir())
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if musicExts[strings.ToLower(filepath.Ext(e.Name()))] {
				out = append(out, musicItem{File: e.Name(), Label: e.Name()})
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
		json.NewEncoder(w).Encode(out)
	}
}

// ─── POST /api/delete-music {"music":"<name>"} ──────────────────────────────

func deleteMusicHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}
		var body struct {
			Music string `json:"music"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		base := filepath.Base(strings.TrimSpace(body.Music))
		if base == "" || base == "." || base == ".." {
			writeJSONErr(w, 400, "tên file không hợp lệ")
			return
		}
		os.Remove(filepath.Join(musicDir(), base))
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}
