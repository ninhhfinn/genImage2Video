package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─── Đọc Excel (.xlsx) cho thumbnail: cột A = nhãn (chữ), cột ảnh = ảnh NHÚNG ───
//
// Người dùng lưu ảnh bằng cách chèn ảnh thẳng vào ô Excel (floating picture) — file
// .xlsx là 1 zip, mỗi ảnh nằm ở xl/media/imageN.*. Ta gom TẤT CẢ ảnh nhúng làm kho
// ảnh, và đọc chữ ở cột A làm kho nhãn. Mỗi thumbnail bốc ngẫu nhiên 4 ảnh + 4 nhãn.

type xlSST struct {
	SI []struct {
		T string `xml:"t"`
		R []struct {
			T string `xml:"t"`
		} `xml:"r"`
	} `xml:"si"`
}

type xlSheet struct {
	Rows []struct {
		R string `xml:"r,attr"`
		C []struct {
			Ref string `xml:"r,attr"`
			T   string `xml:"t,attr"`
			V   string `xml:"v"`
			IS  struct {
				T string `xml:"t"`
			} `xml:"is"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

// readXlsxLabelsAndImages phân tích 1 file .xlsx trong bộ nhớ, trả kho nhãn (chữ
// cột A, bỏ dòng tiêu đề) và kho ảnh (mọi ảnh nhúng trong xl/media).
func readXlsxLabelsAndImages(data []byte) (labels []string, images [][]byte, err error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("không mở được file Excel (.xlsx): %v", err)
	}

	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}

	// 1) sharedStrings (bảng chữ dùng chung)
	var shared []string
	if f := files["xl/sharedStrings.xml"]; f != nil {
		if raw, e := readZipFile(f); e == nil {
			var sst xlSST
			if xml.Unmarshal(raw, &sst) == nil {
				for _, si := range sst.SI {
					if si.T != "" {
						shared = append(shared, si.T)
						continue
					}
					var b strings.Builder
					for _, r := range si.R {
						b.WriteString(r.T)
					}
					shared = append(shared, b.String())
				}
			}
		}
	}

	// 2) worksheet đầu tiên (sheet1.xml hoặc số nhỏ nhất) → nhãn ở cột A, bỏ dòng 1.
	var sheetNames []string
	for name := range files {
		if strings.HasPrefix(name, "xl/worksheets/") && strings.HasSuffix(name, ".xml") &&
			strings.Contains(path.Base(name), "sheet") {
			sheetNames = append(sheetNames, name)
		}
	}
	sort.Strings(sheetNames)
	if len(sheetNames) > 0 {
		if raw, e := readZipFile(files[sheetNames[0]]); e == nil {
			var sh xlSheet
			if xml.Unmarshal(raw, &sh) == nil {
				for _, row := range sh.Rows {
					if rn, _ := strconv.Atoi(row.R); rn <= 1 {
						continue // bỏ dòng tiêu đề
					}
					for _, c := range row.C {
						if colLetters(c.Ref) != "A" {
							continue
						}
						val := ""
						switch c.T {
						case "s": // shared string
							if idx, e := strconv.Atoi(strings.TrimSpace(c.V)); e == nil && idx >= 0 && idx < len(shared) {
								val = shared[idx]
							}
						case "inlineStr":
							val = c.IS.T
						default:
							val = c.V
						}
						if val = strings.TrimSpace(val); val != "" {
							labels = append(labels, val)
						}
					}
				}
			}
		}
	}

	// 3) ảnh nhúng: mọi file trong xl/media/ (jpg/png/webp), sắp tên cho ổn định.
	var mediaNames []string
	for name := range files {
		if strings.HasPrefix(name, "xl/media/") {
			ext := strings.ToLower(filepath.Ext(name))
			if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
				mediaNames = append(mediaNames, name)
			}
		}
	}
	sort.Strings(mediaNames)
	for _, name := range mediaNames {
		if raw, e := readZipFile(files[name]); e == nil && len(raw) > 0 {
			images = append(images, raw)
		}
	}
	return labels, images, nil
}

// readXlsxSerifScript đọc "file chữ": cột A = dòng serif, cột B = script (bỏ dòng 1
// tiêu đề). Trả 2 danh sách để bốc ngẫu nhiên.
func readXlsxSerifScript(data []byte) (serif, script []string, err error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("không mở được file Excel chữ (.xlsx): %v", err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	var shared []string
	if f := files["xl/sharedStrings.xml"]; f != nil {
		if raw, e := readZipFile(f); e == nil {
			var sst xlSST
			if xml.Unmarshal(raw, &sst) == nil {
				for _, si := range sst.SI {
					if si.T != "" {
						shared = append(shared, si.T)
						continue
					}
					var b strings.Builder
					for _, r := range si.R {
						b.WriteString(r.T)
					}
					shared = append(shared, b.String())
				}
			}
		}
	}
	var sheetNames []string
	for name := range files {
		if strings.HasPrefix(name, "xl/worksheets/") && strings.HasSuffix(name, ".xml") &&
			strings.Contains(path.Base(name), "sheet") {
			sheetNames = append(sheetNames, name)
		}
	}
	sort.Strings(sheetNames)
	if len(sheetNames) == 0 {
		return nil, nil, nil
	}
	raw, e := readZipFile(files[sheetNames[0]])
	if e != nil {
		return nil, nil, e
	}
	var sh xlSheet
	if xml.Unmarshal(raw, &sh) != nil {
		return nil, nil, nil
	}
	for _, row := range sh.Rows {
		if rn, _ := strconv.Atoi(row.R); rn <= 1 {
			continue // bỏ dòng tiêu đề
		}
		for _, c := range row.C {
			val := ""
			switch c.T {
			case "s":
				if idx, e := strconv.Atoi(strings.TrimSpace(c.V)); e == nil && idx >= 0 && idx < len(shared) {
					val = shared[idx]
				}
			case "inlineStr":
				val = c.IS.T
			default:
				val = c.V
			}
			if val = strings.TrimSpace(val); val == "" {
				continue
			}
			switch colLetters(c.Ref) {
			case "A":
				serif = append(serif, val)
			case "B":
				script = append(script, val)
			}
		}
	}
	return serif, script, nil
}

// savedThumbImagesPath = nơi lưu "file ảnh + nhãn" để dùng lại nhiều lần.
func savedThumbImagesPath(outputDir string) string {
	return filepath.Join(outputDir, "thumb_images_saved.xlsx")
}

// saveThumbImagesHandler lưu file ảnh+nhãn lên đĩa (POST multipart "excel").
func saveThumbImagesHandler(outputDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			// Trạng thái: đã có file ảnh lưu chưa (để frontend biết lúc mở app).
			if raw, e := os.ReadFile(savedThumbImagesPath(outputDir)); e == nil {
				if labels, images, e2 := readXlsxLabelsAndImages(raw); e2 == nil && len(images) > 0 {
					json.NewEncoder(w).Encode(map[string]interface{}{"saved": true, "images": len(images), "labels": len(labels)})
					return
				}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"saved": false})
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}
		if err := r.ParseMultipartForm(128 << 20); err != nil {
			thumbJSONErr(w, "form hỏng hoặc file quá lớn", 400)
			return
		}
		file, _, err := r.FormFile("excel")
		if err != nil {
			thumbJSONErr(w, "thiếu file Excel (field 'excel')", 400)
			return
		}
		defer file.Close()
		raw, err := io.ReadAll(io.LimitReader(file, 128<<20))
		if err != nil || len(raw) == 0 {
			thumbJSONErr(w, "không đọc được file Excel", 400)
			return
		}
		labels, images, err := readXlsxLabelsAndImages(raw)
		if err != nil {
			thumbJSONErr(w, err.Error(), 400)
			return
		}
		if len(images) == 0 {
			thumbJSONErr(w, "Excel không có ảnh nhúng nào", 400)
			return
		}
		os.MkdirAll(outputDir, 0755)
		if err := os.WriteFile(savedThumbImagesPath(outputDir), raw, 0644); err != nil {
			thumbJSONErr(w, "không lưu được file: "+err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok", "images": len(images), "labels": len(labels),
		})
	}
}

// slashAmenitiesFromName tách tiện ích từ tên phòng theo "/", GIỮ NGUYÊN cả cụm,
// bỏ đoạn đầu (mã phòng/loại). Bản Go của slashAmenities() ở frontend — dùng khi
// frontend không gửi (chọn phòng qua dropdown).
func slashAmenitiesFromName(name string) []string {
	var out []string
	for i, p := range strings.FieldsFunc(name, func(r rune) bool { return r == '|' || r == '/' }) {
		_ = i
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	// bỏ đoạn đầu (mã phòng/loại) nếu có ≥2 đoạn
	if len(out) >= 2 {
		out = out[1:]
	} else {
		out = nil
	}
	return out
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// firstNonEmpty trả chuỗi non-rỗng đầu tiên.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// colLetters lấy phần chữ cái đầu của tham chiếu ô ("B12" → "B").
func colLetters(ref string) string {
	var b strings.Builder
	for _, c := range ref {
		if c >= 'A' && c <= 'Z' {
			b.WriteRune(c)
		} else {
			break
		}
	}
	return b.String()
}

// pickRandomImages bốc n ảnh (lặp lại nếu kho < n).
func pickRandomImages(pool [][]byte, n int) [][]byte {
	if len(pool) == 0 || n <= 0 {
		return nil
	}
	perm := rand.Perm(len(pool))
	out := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, pool[perm[i%len(perm)]])
	}
	return out
}

// pickRandomStrings bốc n chuỗi (lặp lại nếu kho < n, rỗng nếu kho trống).
func pickRandomStrings(pool []string, n int) []string {
	if len(pool) == 0 || n <= 0 {
		return nil
	}
	perm := rand.Perm(len(pool))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, pool[perm[i%len(perm)]])
	}
	return out
}

// ─── POST /api/render-thumbnail-excel ───────────────────────────────────────
// multipart: field "excel" (.xlsx), "address" (địa chỉ listing → suy ra tỉnh),
// "template" (mặc định "valentine"). Trả {status,url} như /api/render-thumbnail.
func excelThumbnailHandler(outputDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			thumbJSONErr(w, "form hỏng hoặc file quá lớn", 400)
			return
		}
		// File ảnh+nhãn: ưu tiên file gửi lên (và LƯU lại để lần sau khỏi nhập);
		// nếu không gửi → dùng file đã lưu trên server.
		var raw []byte
		if file, _, e := r.FormFile("excel"); e == nil {
			defer file.Close()
			raw, _ = io.ReadAll(io.LimitReader(file, 128<<20))
			if len(raw) > 0 {
				os.MkdirAll(outputDir, 0755)
				os.WriteFile(savedThumbImagesPath(outputDir), raw, 0644)
			}
		}
		if len(raw) == 0 {
			if b, e := os.ReadFile(savedThumbImagesPath(outputDir)); e == nil {
				raw = b
			}
		}
		if len(raw) == 0 {
			thumbJSONErr(w, "chưa có file ảnh+nhãn — hãy lưu file ảnh trước", 400)
			return
		}

		labels, images, err := readXlsxLabelsAndImages(raw)
		if err != nil {
			thumbJSONErr(w, err.Error(), 400)
			return
		}
		if len(images) == 0 {
			thumbJSONErr(w, "Excel không có ảnh nhúng nào (hãy chèn ảnh vào cột ảnh)", 400)
			return
		}

		// File chữ (serif+script): nếu gửi → bốc ngẫu nhiên serif (cột A) + script (cột B).
		var textSerif, textScript string
		if tf, _, e := r.FormFile("excel_text"); e == nil {
			defer tf.Close()
			if traw, e2 := io.ReadAll(io.LimitReader(tf, 16<<20)); e2 == nil && len(traw) > 0 {
				if sList, scList, e3 := readXlsxSerifScript(traw); e3 == nil {
					if s := pickRandomStrings(sList, 1); len(s) > 0 {
						textSerif = s[0]
					}
					if s := pickRandomStrings(scList, 1); len(s) > 0 {
						textScript = s[0]
					}
				}
			}
		}

		// Lưu 4 ảnh ngẫu nhiên ra file tạm để Chrome nạp qua file://.
		picks := pickRandomImages(images, 4)
		tmpDir, err := os.MkdirTemp("", "xlsxthumb")
		if err != nil {
			thumbJSONErr(w, "không tạo được thư mục tạm", 500)
			return
		}
		defer os.RemoveAll(tmpDir)
		var photos []string
		for i, b := range picks {
			p := filepath.Join(tmpDir, fmt.Sprintf("img%d.jpg", i))
			if e := os.WriteFile(p, b, 0o644); e == nil {
				photos = append(photos, p)
			}
		}
		if len(photos) == 0 {
			thumbJSONErr(w, "không ghi được ảnh tạm", 500)
			return
		}

		template := strings.TrimSpace(r.FormValue("template"))
		if template == "" {
			template = "valentine"
		}

		// address + tiện ích: ưu tiên frontend gửi; nếu rỗng (chọn phòng qua dropdown
		// không set activeListing) thì DỰ PHÒNG lấy từ currentListing của backend.
		address := strings.TrimSpace(r.FormValue("address"))
		var listingAmen []string
		if r.MultipartForm != nil {
			for _, a := range r.MultipartForm.Value["amenity"] {
				if a = strings.TrimSpace(a); a != "" {
					listingAmen = append(listingAmen, a)
				}
			}
		}
		currentListing.mu.Lock()
		clName, clAddr := currentListing.Name, currentListing.Addr
		currentListing.mu.Unlock()
		if address == "" {
			address = strings.TrimSpace(clAddr)
		}
		if len(listingAmen) == 0 {
			listingAmen = slashAmenitiesFromName(clName)
		}

		// 4 nhãn = 1–2 tiện ích THẬT của phòng + phần còn lại bốc từ cột nhãn Excel.
		nFromListing := 2
		if len(listingAmen) < nFromListing {
			nFromListing = len(listingAmen)
		}
		tags := pickRandomStrings(listingAmen, nFromListing)
		if need := 4 - len(tags); need > 0 {
			tags = append(tags, pickRandomStrings(labels, need)...)
		}
		// trộn để tiện ích phòng không luôn nằm cùng vị trí
		rand.Shuffle(len(tags), func(i, j int) { tags[i], tags[j] = tags[j], tags[i] })

		cfg := ThumbnailConfig{
			Template:  template,
			Address:   address,
			Amenities: tags,
			ValBadge:  strings.TrimSpace(r.FormValue("badge")),
			ValTitle:  firstNonEmpty(textSerif, strings.TrimSpace(r.FormValue("title_line"))),
			ValScript: firstNonEmpty(textScript, strings.TrimSpace(r.FormValue("script_line"))),
		}
		data, err := buildThumbnailImage(cfg, photos)
		if err != nil {
			thumbJSONErr(w, err.Error(), 500)
			return
		}
		os.MkdirAll(outputDir, 0755)
		if err := os.WriteFile(filepath.Join(outputDir, "thumbnail.jpg"), data, 0644); err != nil {
			thumbJSONErr(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"url":     fmt.Sprintf("/api/thumbnail-file?ts=%d", time.Now().UnixNano()),
			"size":    len(data),
			"images":  len(images),
			"labels":  len(labels),
			"picked":  len(photos),
		})
	}
}
