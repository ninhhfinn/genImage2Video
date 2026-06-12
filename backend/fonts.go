package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"img2video/fontprep"
	"img2video/textrender"
)

// Bring-your-own fonts.
//
// Uploaded fonts are validated + (if variable) instanced to a static Bold
// master by the fontprep package, then stored under assets/fonts/uploads/ so
// the textrender pipeline can reference them by the relative name
// "uploads/<file>" — exactly like any shipped font. They flow into both the
// video title overlay (via TitleFontFile → applyConfigOverrides) and the
// thumbnail (title_font), and the preview endpoint renders them through the
// real gg pipeline so what the user sees is what gen-video produces.

const (
	maxFontBytes      = 8 << 20 // 8 MB
	uploadsSubdir     = "uploads"
	defaultPreviewTxt = "Biệt thự 3 tầng – 120m²\nĐường Nguyễn Huệ, Quận 1"
)

// FontMeta describes one uploaded font in the manifest.
type FontMeta struct {
	File              string   `json:"file"`  // relative to fonts/, e.g. "uploads/user_ab12cd34.ttf"
	Label             string   `json:"label"` // original filename, for display
	Family            string   `json:"family,omitempty"`
	Source            string   `json:"source"`  // "uploaded" | "builtin"
	Variable          bool     `json:"variable"` // was the upload a variable font
	Instanced         bool     `json:"instanced"`
	VietnameseOK      bool     `json:"vietnamese_ok"`
	MissingVietnamese string   `json:"missing_vietnamese,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
}

var manifestMu sync.Mutex

func userFontsDir() string { return filepath.Join(assetsDir(), "fonts", uploadsSubdir) }

func manifestPath() string { return filepath.Join(userFontsDir(), "manifest.json") }

func loadManifest() map[string]FontMeta {
	m := map[string]FontMeta{}
	data, err := os.ReadFile(manifestPath())
	if err != nil {
		return m
	}
	_ = json.Unmarshal(data, &m)
	return m
}

func saveManifest(m map[string]FontMeta) error {
	if err := os.MkdirAll(userFontsDir(), 0755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(manifestPath(), data, 0644)
}

// safeFontName derives a collision-free on-disk name from the content hash plus
// a sanitized stem of the original filename. User input never touches the path.
func safeFontName(original string, content []byte) string {
	stem := strings.TrimSuffix(filepath.Base(original), filepath.Ext(original))
	var b strings.Builder
	for _, r := range strings.ToLower(stem) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	clean := strings.Trim(b.String(), "-")
	if len(clean) > 24 {
		clean = clean[:24]
	}
	if clean == "" {
		clean = "font"
	}
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%s-%s.ttf", clean, hex.EncodeToString(sum[:])[:8])
}

// ─── POST /api/upload-font ───────────────────────────────────────────────────

func uploadFontHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxFontBytes+(1<<20))
		if err := r.ParseMultipartForm(maxFontBytes + (1 << 20)); err != nil {
			writeJSONErr(w, 400, "file quá lớn hoặc form hỏng (tối đa 8MB)")
			return
		}
		file, hdr, err := r.FormFile("font")
		if err != nil {
			writeJSONErr(w, 400, "thiếu field 'font'")
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		if ext != ".ttf" && ext != ".otf" {
			writeJSONErr(w, 400, "chỉ nhận .ttf hoặc .otf")
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, maxFontBytes+1))
		if err != nil || len(data) == 0 {
			writeJSONErr(w, 400, "không đọc được file")
			return
		}
		if len(data) > maxFontBytes {
			writeJSONErr(w, 400, "file quá lớn (tối đa 8MB)")
			return
		}

		// Validate + (if variable) instance to a static Bold master.
		finalData, rep, instanced, prepErr := fontprep.Prepare(data)
		if prepErr != nil {
			writeJSONErr(w, 422, prepErr.Error())
			return
		}

		name := safeFontName(hdr.Filename, data) // hash original bytes for stable id
		if err := os.MkdirAll(userFontsDir(), 0755); err != nil {
			writeJSONErr(w, 500, "không tạo được thư mục fonts")
			return
		}
		if err := os.WriteFile(filepath.Join(userFontsDir(), name), finalData, 0644); err != nil {
			writeJSONErr(w, 500, "không lưu được font")
			return
		}

		rel := uploadsSubdir + "/" + name
		meta := FontMeta{
			File:              rel,
			Label:             hdr.Filename,
			Family:            rep.Family,
			Source:            "uploaded",
			Variable:          instanced, // true means it WAS variable and we fixed it
			Instanced:         instanced,
			VietnameseOK:      rep.VietnameseOK,
			MissingVietnamese: rep.MissingVietnamese,
			Warnings:          rep.Warnings,
		}

		manifestMu.Lock()
		m := loadManifest()
		m[rel] = meta
		saveErr := saveManifest(m)
		manifestMu.Unlock()
		if saveErr != nil {
			writeJSONErr(w, 500, "không ghi được manifest")
			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"ok":    true,
			"font":  meta,
			"report": rep,
		})
	}
}

// ─── GET /api/fonts ───────────────────────────────────────────────────────────
// Lists fonts the picker can offer: shipped fonts (excluding the variable
// originals we don't want chosen directly) + every uploaded font.

func listFontsHandler() http.HandlerFunc {
	// Shipped fonts worth offering as title fonts, with friendly labels.
	builtin := []FontMeta{
		{File: "Baloo2-Bold.ttf", Label: "Baloo 2 (tròn, đậm)", Source: "builtin", VietnameseOK: true},
		{File: "Baloo2-ExtraBold.ttf", Label: "Baloo 2 (cực đậm)", Source: "builtin", VietnameseOK: true},
		{File: "BeVietnamPro-Bold.ttf", Label: "Be Vietnam Pro (đậm)", Source: "builtin", VietnameseOK: true},
		{File: "Prata-Regular.ttf", Label: "Prata (serif)", Source: "builtin", VietnameseOK: true},
		{File: "PlayfairDisplay-Bold.ttf", Label: "Playfair Display (serif đậm)", Source: "builtin", VietnameseOK: true},
		{File: "YesevaOne-Regular.ttf", Label: "Yeseva One (serif cong)", Source: "builtin", VietnameseOK: true},
		{File: "LilitaOne-Regular.ttf", Label: "Lilita One (dày)", Source: "builtin", VietnameseOK: true},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		out := []FontMeta{}
		fontsDir := filepath.Join(assetsDir(), "fonts")
		for _, b := range builtin {
			if _, err := os.Stat(filepath.Join(fontsDir, b.File)); err == nil {
				out = append(out, b)
			}
		}

		manifestMu.Lock()
		m := loadManifest()
		manifestMu.Unlock()
		uploaded := make([]FontMeta, 0, len(m))
		for _, meta := range m {
			// Drop manifest entries whose file vanished from disk.
			if _, err := os.Stat(filepath.Join(fontsDir, filepath.FromSlash(meta.File))); err == nil {
				uploaded = append(uploaded, meta)
			}
		}
		sort.Slice(uploaded, func(i, j int) bool { return uploaded[i].Label < uploaded[j].Label })
		out = append(out, uploaded...)

		json.NewEncoder(w).Encode(out)
	}
}

// ─── GET /api/font-preview?font=<file>&text=<...> ─────────────────────────────
// Renders the font through the REAL gg pipeline and returns a PNG. This is the
// faithful preview: variable-thin and tofu show up here exactly as they would
// in the final video.

func fontPreviewHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fontFile, err := resolveFontFile(r.URL.Query().Get("font"))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		text := strings.TrimSpace(r.URL.Query().Get("text"))
		if text == "" {
			text = defaultPreviewTxt
		}
		if len([]rune(text)) > 80 {
			text = string([]rune(text)[:80])
		}

		style := &textrender.ElementStyle{
			Text:     text,
			FontFile: fontFile,
			SizePct:  0.06,
			Color:    "#FFFFFF",
			Bg:       &textrender.BgStyle{Color: "#11151c", Alpha: 0.92, Radius: 28, Padding: [2]float64{28, 56}},
			Position: textrender.Position{X: "center", Y: "center"},
			Align:    "center", LineSpacing: 1.3, MaxWidthPct: 0.9,
		}
		ctx := &textrender.RenderContext{VideoWidth: 1080, VideoHeight: 1350, AssetsDir: assetsDir()}
		el, err := textrender.Render(style, ctx)
		if err != nil || el == nil {
			http.Error(w, "render lỗi", 500)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(el.PNG)
	}
}

// ─── POST /api/delete-font {"font":"uploads/x.ttf"} ───────────────────────────

func deleteFontHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body struct {
			Font string `json:"font"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		rel := strings.TrimSpace(body.Font)
		if !strings.HasPrefix(rel, uploadsSubdir+"/") {
			writeJSONErr(w, 400, "chỉ xoá được font tự tải lên")
			return
		}
		// Re-derive the path from the base name only — no traversal possible.
		base := filepath.Base(filepath.FromSlash(rel))
		os.Remove(filepath.Join(userFontsDir(), base))

		manifestMu.Lock()
		m := loadManifest()
		delete(m, rel)
		saveManifest(m)
		manifestMu.Unlock()

		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}

// resolveFontFile validates a font reference from a query param and returns the
// relative FontFile to hand textrender. Prevents path traversal: the result is
// always a shipped file in fonts/ or a base name under fonts/uploads/.
func resolveFontFile(font string) (string, error) {
	font = strings.TrimSpace(font)
	if font == "" {
		return "", fmt.Errorf("thiếu tham số font")
	}
	if strings.Contains(font, "..") {
		return "", fmt.Errorf("font không hợp lệ")
	}
	fontsDir := filepath.Join(assetsDir(), "fonts")
	if strings.HasPrefix(font, uploadsSubdir+"/") {
		base := filepath.Base(filepath.FromSlash(font))
		if _, err := os.Stat(filepath.Join(userFontsDir(), base)); err != nil {
			return "", fmt.Errorf("font không tồn tại")
		}
		return uploadsSubdir + "/" + base, nil
	}
	base := filepath.Base(font)
	if _, err := os.Stat(filepath.Join(fontsDir, base)); err != nil {
		return "", fmt.Errorf("font không tồn tại")
	}
	return base, nil
}

func writeJSONErr(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
