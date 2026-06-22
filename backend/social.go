package main

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SocialMeta là gói thông tin gửi kèm video tới webhook tự-đăng (Make.com / n8n),
// để automation bên đó tự đăng lên TikTok / Facebook.
type SocialMeta struct {
	Title     string   `json:"title"`
	Caption   string   `json:"caption"`
	Nickname  string   `json:"nickname"`
	Address   string   `json:"address"`
	ListingID string   `json:"listing_id"`
	Prices    []string `json:"prices"`
	Amenities []string `json:"amenities"`
	Platforms []string `json:"platforms"` // vd ["tiktok","facebook"]
}

// buildCaption ghép caption thân thiện từ metadata listing. Nếu Caption đã có sẵn
// (người dùng tự nhập) thì dùng nguyên văn.
func buildCaption(m SocialMeta) string {
	if strings.TrimSpace(m.Caption) != "" {
		return m.Caption
	}
	var b strings.Builder
	// dòng tiêu đề đầu tiên không rỗng
	for _, line := range strings.Split(m.Title, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			b.WriteString(s + "\n")
			break
		}
	}
	if s := strings.TrimSpace(m.Address); s != "" {
		b.WriteString("📍 " + s + "\n")
	}
	for _, p := range m.Prices {
		if s := strings.TrimSpace(p); s != "" {
			b.WriteString("💰 " + s + "\n")
		}
	}
	if len(m.Amenities) > 0 {
		b.WriteString("✨ " + strings.Join(m.Amenities, " · ") + "\n")
	}
	if s := strings.TrimSpace(m.ListingID); s != "" {
		b.WriteString("Mã: " + s)
	}
	return strings.TrimSpace(b.String())
}

// postToWebhook tải video kèm metadata listing lên webhook dạng multipart/form-data.
// Bên nhận (Make.com, n8n, …) chịu trách nhiệm đăng lên nền tảng đã chọn.
//
// Body được stream bằng io.Pipe để video lớn không nằm hết trong RAM. Trả về lỗi
// nếu lỗi truyền tải hoặc phản hồi không phải 2xx. Việc đăng là best-effort: caller
// coi lỗi ở đây là không nghiêm trọng, một video render xong vẫn hợp lệ dù gửi lỗi.
func postToWebhook(webhookURL, videoPath string, m SocialMeta) error {
	f, err := os.Open(videoPath)
	if err != nil {
		return fmt.Errorf("mở file video: %w", err)
	}
	defer f.Close()

	m.Caption = buildCaption(m)

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	contentType := mw.FormDataContentType() // boundary cố định từ lúc tạo writer

	go func() {
		var perr error
		defer func() { pw.CloseWithError(perr) }()

		// metadata dạng JSON (cho tool đọc được JSON lồng nhau)
		if metaJSON, e := json.Marshal(m); e == nil {
			mw.WriteField("meta", string(metaJSON))
		}
		// và các field phẳng (cho tool no-code khó parse JSON)
		mw.WriteField("caption", m.Caption)
		mw.WriteField("platforms", strings.Join(m.Platforms, ","))

		part, e := mw.CreateFormFile("video", filepath.Base(videoPath))
		if e != nil {
			perr = e
			return
		}
		if _, e = io.Copy(part, f); e != nil {
			perr = e
			return
		}
		perr = mw.Close()
	}()

	req, err := http.NewRequest("POST", webhookURL, pr)
	if err != nil {
		return fmt.Errorf("tạo request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gửi webhook: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook trả mã %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
