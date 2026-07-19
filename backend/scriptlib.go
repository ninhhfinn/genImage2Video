package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── Thư viện kịch bản + feedback ────────────────────────────────────────────
//
// Mỗi kịch bản dùng để render được lưu lại đây. User đánh dấu "hay" (liked) →
// kịch bản đó tự động thành ví dụ few-shot cho các lần viết sau (thay dần mẫu
// tĩnh). Đây là vòng "huấn luyện dần": càng like nhiều bản chuẩn gu, Claude
// càng bám giọng đó.

const (
	maxScriptLibEntries = 200 // trần thư viện; vượt thì bỏ bản cũ chưa liked
	maxLikedExamples    = 3   // số ví dụ liked tối đa chèn vào prompt
	maxExampleSegments  = 3   // ví dụ dài được nén còn hook/thân/CTA
	maxExamplesInPrompt = 4   // tổng ví dụ (liked + tĩnh) trong prompt
)

// scriptLibDirOverride: test seam — trỏ thư viện sang thư mục tạm.
var scriptLibDirOverride string

var scriptLibMu sync.Mutex

// ScriptEntry = một kịch bản đã dùng render, lưu làm thư viện/feedback.
type ScriptEntry struct {
	ID             string             `json:"id"`
	ListingID      string             `json:"listing_id"`
	Nickname       string             `json:"nickname"`
	Persona        string             `json:"persona"`
	Segments       []NarrationSegment `json:"segments"`
	HookLine1      string             `json:"hook_line1,omitempty"`
	HookLine2      string             `json:"hook_line2,omitempty"`
	HookEmphasis   string             `json:"hook_emphasis,omitempty"`
	IntroNarration string             `json:"intro_narration,omitempty"` // lời hook cảnh đi đường (v6)
	Edited         bool               `json:"edited"`
	Liked          bool               `json:"liked"`
	CreatedAt      string             `json:"created_at"`
}

func scriptLibPath() string {
	dir := scriptLibDirOverride
	if dir == "" {
		dir = filepath.Join(assetsDir(), "scripts")
	}
	return filepath.Join(dir, "library.json")
}

// loadScriptLib đọc toàn bộ thư viện, mới nhất trước. Lỗi đọc/parse → rỗng.
func loadScriptLib() []ScriptEntry {
	scriptLibMu.Lock()
	defer scriptLibMu.Unlock()
	return loadScriptLibLocked()
}

func loadScriptLibLocked() []ScriptEntry {
	data, err := os.ReadFile(scriptLibPath())
	if err != nil {
		return nil
	}
	var entries []ScriptEntry
	if json.Unmarshal(data, &entries) != nil {
		return nil
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].CreatedAt > entries[j].CreatedAt })
	return entries
}

func saveScriptLibLocked(entries []ScriptEntry) error {
	p := scriptLibPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

// scriptEntryID: hash nội dung → cùng kịch bản lưu lại không tạo bản trùng.
func scriptEntryID(persona string, segs []NarrationSegment) string {
	h := sha256.New()
	h.Write([]byte(normalizePersona(persona) + "|"))
	for _, s := range segs {
		h.Write([]byte(s.Caption + "·" + s.Narration + "|"))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func normalizePersona(p string) string {
	if strings.EqualFold(strings.TrimSpace(p), "lichsu") {
		return "lichsu"
	}
	return "haihuoc"
}

// saveScriptEntry upsert một kịch bản vào thư viện (gọi sau khi render dùng nó).
// edited = user đã duyệt/sửa qua panel FE. Lỗi I/O chỉ warn — không chặn render.
func saveScriptEntry(cfg Config, s *NarrationScript, edited bool) {
	if s == nil || len(s.Segments) == 0 {
		return
	}
	scriptLibMu.Lock()
	defer scriptLibMu.Unlock()

	entries := loadScriptLibLocked()
	id := scriptEntryID(cfg.NarrationPersona, s.Segments)
	for i := range entries {
		if entries[i].ID == id {
			entries[i].Edited = entries[i].Edited || edited // giữ Liked cũ
			if err := saveScriptLibLocked(entries); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Không lưu được thư viện kịch bản: %v\n", err)
			}
			return
		}
	}
	entries = append([]ScriptEntry{{
		ID:             id,
		ListingID:      cfg.ListingID,
		Nickname:       cfg.Nickname,
		Persona:        normalizePersona(cfg.NarrationPersona),
		Segments:       s.Segments,
		HookLine1:      s.HookLine1,
		HookLine2:      s.HookLine2,
		HookEmphasis:   s.HookEmphasis,
		IntroNarration: s.IntroNarration,
		Edited:         edited,
		CreatedAt:      time.Now().Format(time.RFC3339),
	}}, entries...)

	// Trần thư viện: bỏ bản CŨ NHẤT chưa liked trước.
	for len(entries) > maxScriptLibEntries {
		dropped := false
		for i := len(entries) - 1; i >= 0; i-- {
			if !entries[i].Liked {
				entries = append(entries[:i], entries[i+1:]...)
				dropped = true
				break
			}
		}
		if !dropped { // toàn liked → đành bỏ bản cũ nhất
			entries = entries[:len(entries)-1]
		}
	}
	if err := saveScriptLibLocked(entries); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Không lưu được thư viện kịch bản: %v\n", err)
	}
}

func setScriptLiked(id string, liked bool) error {
	scriptLibMu.Lock()
	defer scriptLibMu.Unlock()
	entries := loadScriptLibLocked()
	for i := range entries {
		if entries[i].ID == id {
			entries[i].Liked = liked
			return saveScriptLibLocked(entries)
		}
	}
	return fmt.Errorf("không thấy kịch bản %q", id)
}

func deleteScriptEntry(id string) error {
	scriptLibMu.Lock()
	defer scriptLibMu.Unlock()
	entries := loadScriptLibLocked()
	for i := range entries {
		if entries[i].ID == id {
			return saveScriptLibLocked(append(entries[:i], entries[i+1:]...))
		}
	}
	return fmt.Errorf("không thấy kịch bản %q", id)
}

// likedScriptExamples: các kịch bản đã liked (persona khớp, mới nhất trước)
// chuyển thành ví dụ few-shot. Kịch bản dài nén còn hook / một đoạn thân / CTA
// (image_index đánh lại 0..n) cho đỡ tốn prompt.
func likedScriptExamples(persona string, max int) []string {
	persona = normalizePersona(persona)
	var out []string
	for _, e := range loadScriptLib() {
		if !e.Liked || normalizePersona(e.Persona) != persona {
			continue
		}
		segs := e.Segments
		if len(segs) > maxExampleSegments {
			segs = []NarrationSegment{segs[0], segs[len(segs)/2], segs[len(segs)-1]}
		}
		compact := make([]NarrationSegment, len(segs))
		for i, s := range segs {
			s.ImageIndex = i
			compact[i] = s
		}
		data, err := json.Marshal(NarrationScript{Segments: compact, IntroNarration: e.IntroNarration})
		if err != nil {
			continue
		}
		out = append(out, string(data))
		if len(out) >= max {
			break
		}
	}
	return out
}

// narrationExamplesHash: hash của khối ví dụ đang dùng — nằm trong cache key
// kịch bản để đổi ví dụ (like/bỏ like) là cache cũ hết tác dụng.
func narrationExamplesHash(persona string) string {
	sum := sha256.Sum256([]byte(narrationExamplesBlock(persona)))
	return fmt.Sprintf("%x", sum[:8])
}

// ─── HTTP: GET /api/scripts · POST /api/like-script · POST /api/delete-script ─

func listScriptsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		entries := loadScriptLib()
		if entries == nil {
			entries = []ScriptEntry{}
		}
		json.NewEncoder(w).Encode(entries)
	}
}

func likeScriptHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}
		var body struct {
			ID    string `json:"id"`
			Liked bool   `json:"liked"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if err := setScriptLiked(body.ID, body.Liked); err != nil {
			writeJSONErr(w, 404, err.Error())
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}

func deleteScriptHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}
		var body struct {
			ID string `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if err := deleteScriptEntry(body.ID); err != nil {
			writeJSONErr(w, 404, err.Error())
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}
