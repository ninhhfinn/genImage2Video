package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Render history ───────────────────────────────────────────────────────

type RenderRecord struct {
	ListingName string  `json:"listing_name"`
	ListingID   string  `json:"listing_id"`
	Address     string  `json:"address"`
	PhotoCount  int     `json:"photo_count"`
	Duration    float64 `json:"duration"`
	FileSizeMB  float64 `json:"file_size_mb"`
	RenderTime  string  `json:"render_time"`
	FileName    string  `json:"filename"`
	Note        string  `json:"note"`
}

var (
	renderHistory []RenderRecord
	historyMu     sync.Mutex
)

func addToHistory(rec RenderRecord) {
	historyMu.Lock()
	defer historyMu.Unlock()
	renderHistory = append(renderHistory, rec)
}

// ─── Current listing context ──────────────────────────────────────────────

type ListingContext struct {
	mu     sync.Mutex
	Name   string
	ID     string
	Addr   string
	Photos int
}

var currentListing = &ListingContext{}

func setCurrentListing(name, id, addr string, photos int) {
	currentListing.mu.Lock()
	defer currentListing.mu.Unlock()
	currentListing.Name   = name
	currentListing.ID     = id
	currentListing.Addr   = addr
	currentListing.Photos = photos
}

// ─── Export Excel (pure Go) ───────────────────────────────────────────────

func exportExcelHandler(outputDir string, port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		historyMu.Lock()
		records := make([]RenderRecord, len(renderHistory))
		copy(records, renderHistory)
		historyMu.Unlock()

		if len(records) == 0 {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"chưa có video nào được render"}`, 400)
			return
		}

		baseURL := fmt.Sprintf("http://localhost:%d", port)
		data, err := buildExcelFromHistory(records, baseURL)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), 500)
			return
		}

		w.Header().Set("Content-Disposition", `attachment; filename="videos_export.xlsx"`)
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write(data)
	}
}

// ─── Video file server ────────────────────────────────────────────────────

func videoFileHandler(outputDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filename := filepath.Base(r.URL.Path)
		fpath := filepath.Join(outputDir, filename)
		if _, err := os.Stat(fpath); err != nil {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeFile(w, r, fpath)
	}
}

// ─── History endpoint ─────────────────────────────────────────────────────

func historyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		historyMu.Lock()
		records := make([]RenderRecord, len(renderHistory))
		copy(records, renderHistory)
		historyMu.Unlock()

		type row struct {
			RenderRecord
			DownloadURL string `json:"download_url"` // path tương đối — cùng origin với web (dev proxy / prod)
		}
		rows := make([]row, len(records))
		for i, rec := range records {
			rows[i] = row{rec, "/api/video/" + rec.FileName}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"records": rows,
			"count":   len(rows),
		})
	}
}

// ─── Save video with unique filename ─────────────────────────────────────

func saveVideoFile(srcPath, outputDir, listingID string) (string, error) {
	ts   := time.Now().Format("20060102_150405")
	name := fmt.Sprintf("video_%s.mp4", ts)
	if listingID != "" {
		name = fmt.Sprintf("%s_%s.mp4", sanitizeFilename(listingID), ts)
	}
	dst := filepath.Join(outputDir, name)

	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	d, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer d.Close()

	buf := make([]byte, 4<<20)
	for {
		n, e := src.Read(buf)
		if n > 0 {
			d.Write(buf[:n])
		}
		if e != nil {
			break
		}
	}
	return name, nil
}

func sanitizeFilename(s string) string {
	out := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return string(out)
}
