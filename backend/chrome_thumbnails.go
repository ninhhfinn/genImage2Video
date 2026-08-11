package main

// 6 template THUMBNAIL "canva1".."canva6" dựng lại từ Canva design DAHL8ohTEjE
// ("Template ảnh", 6 trang 1080×1920), render bằng headless Chrome giống
// chrome_overlay.go nhưng ẢNH THẬT nằm trong trang (<img object-fit:cover>)
// thay vì composite ffmpeg. Spec vị trí/cỡ/màu đã match từng trang qua vòng lặp
// render→compare→fix (scratchpad thumbloop, 2026-07-04). Font thay thế cho font
// Canva trả phí: Garet→Poppins, Have Heart→Kaushan Script, Comic Sans→Comic
// Neue (fallback Baloo 2 cho glyph Việt), chữ tay "homestay"→Pacifico.

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/jpeg"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/disintegration/imaging"
)

// dataFontFace nhúng 1 file font thành @font-face base64 (data URI). Dùng cho font
// hay bị headless Chrome chụp-trước-khi-nạp qua file:// (VD Parisienne): data URI có
// sẵn ngay lúc layout nên KHÔNG còn đua tải → hết rớt về Comic Sans.
func dataFontFace(family, file string) string {
	b, err := os.ReadFile(filepath.Join(assetsDir(), "fonts", file))
	if err != nil {
		return ""
	}
	enc := base64.StdEncoding.EncodeToString(b)
	return fmt.Sprintf("@font-face{font-family:'%s';src:url('data:font/ttf;base64,%s') format('truetype');font-weight:400;font-style:normal;font-display:block}", family, enc)
}

// valentineFontCSS trả @font-face base64 cho font riêng của template "valentine"
// (chỉ Parisienne — Playfair/Be Vietnam Pro nạp file:// đã ổn).
func valentineFontCSS() string {
	return dataFontFace("ParisienneVB", "Parisienne-Regular.ttf")
}

// chromeThumbNames — template thumbnail render bằng Chrome.
var chromeThumbNames = map[string]bool{
	"canva1": true, "canva2": true, "canva3": true,
	"canva4": true, "canva5": true, "canva6": true,
	"valentine": true,
}

// thumbStageWH trả kích thước khung render cho từng template. Canva mặc định 9:16
// (1080×1920); "valentine" dùng 3:4 (1080×1440) đúng như bản thiết kế đã duyệt.
func thumbStageWH(name string) (int, int) {
	if strings.ToLower(strings.TrimSpace(name)) == "valentine" {
		return 1080, 1440
	}
	return 1080, 1920
}

// thumbShortAddress rút địa chỉ gọn cho thumbnail (mockup: "Đường Nguyễn Khang,
// Cầu Giấy" = phố + quận, KHÔNG kèm tỉnh/thành).
func thumbShortAddress(addr string) string {
	var parts []string
	for _, p := range strings.Split(cleanVideoAddress(addr), ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if n := len(parts); n >= 2 && isProvinceName(parts[n-1]) {
		parts = parts[:n-1]
	}
	return strings.Join(parts, ", ")
}

// thumbImg dựng <img> ảnh thật phủ kín vùng (crop giữa như imaging.Fill).
func thumbImg(path, pos string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return fmt.Sprintf(`<img src="file://%s" style="position:absolute;%s;object-fit:cover" />`, abs, pos)
}

// thumbPhoto trả ảnh thứ i, lặp lại ảnh cuối khi thiếu.
func thumbPhoto(photos []string, i int) string {
	if len(photos) == 0 {
		return ""
	}
	if i >= len(photos) {
		i = len(photos) - 1
	}
	return photos[i]
}

// thumbVeil phủ tối toàn trang (mockup Canva T1/T2 tối ấm rõ, các trang khác nhẹ).
func thumbVeil(rgba string) string {
	return `<div style="position:absolute;inset:0;background:` + rgba + `"></div>`
}

// thumbFitLines nén line-height (và cỡ chữ nếu leading bẹt dưới 1.25) cho khối
// giá n dòng phải lọt trong budget px — spec mockup chỉ tính cho ít dòng, API
// thật gửi tới 5 dòng giá. n nhỏ → trả đúng base (giữ nguyên parity mockup).
func thumbFitLines(budget, baseLH, baseFS, n int) (lh, fs int) {
	lh, fs = baseLH, baseFS
	if n > 0 && n*lh > budget {
		lh = budget / n
		if f := int(math.Round(float64(lh) / 1.25)); f < fs {
			fs = f
		}
	}
	return
}

// thumbLineCount đếm số dòng của chuỗi đã escLines (\n-joined).
func thumbLineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// arcAddrFont ước lượng cỡ chữ Yeseva trên cung địa chỉ canva1 để không tràn
// (mockup 28 rune @41.7px trên dây cung ~880px; ~0.6em/rune).
func arcAddrFont(addr string) float64 {
	n := utf8.RuneCountInString(addr)
	if n <= 0 {
		return 41.7
	}
	if f := 880.0 / (0.60 * float64(n)); f < 41.7 {
		return f
	}
	return 41.7
}

// chromeThumbBody dựng phần thân HTML của template thumbnail.
func chromeThumbBody(name string, cfg ThumbnailConfig, photos []string) string {
	title := esc(strings.TrimSpace(cfg.Title))
	addr := esc(thumbShortAddress(cfg.Address))
	prices := escLines(trimNonEmpty(cfg.Prices))
	amen := esc(strings.Join(trimNonEmpty(cfg.Amenities), ", "))

	var b strings.Builder
	switch name {

	case "canva1": // Trang 1 — 3 dải ảnh dọc, chữ Yeseva trắng, địa chỉ cong
		b.WriteString(thumbImg(thumbPhoto(photos, 0), "top:0;left:0;width:1080px;height:640px"))
		b.WriteString(thumbImg(thumbPhoto(photos, 1), "top:643px;left:0;width:1080px;height:640px"))
		b.WriteString(thumbImg(thumbPhoto(photos, 2), "top:1286px;left:0;width:1080px;height:634px"))
		b.WriteString(thumbVeil("rgba(28,14,7,.42)"))
		b.WriteString(fmt.Sprintf(`<svg style="position:absolute;top:0;left:0" width="1080" height="800" viewBox="0 0 1080 800"><defs><path id="addrArc" d="M 99.5,664 A 1060,1060 0 0 1 979.5,664" fill="none"/></defs><text font-family="'Yeseva One',serif" font-size="%.1f" fill="#ffffff"><textPath href="#addrArc" startOffset="50%%" text-anchor="middle">%s</textPath></text></svg>`, arcAddrFont(addr), addr))
		b.WriteString(el("top:636px;left:0;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:89px;color:#ffffff;"+thumbShSoft, fitSpan(1000, title)))
		caption := strings.TrimSpace(cfg.Caption)
		if caption == "" {
			caption = "Trạm sạc cảm xúc"
		}
		b.WriteString(el("top:746px;left:9px;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:43px;color:#ffffff;"+thumbShSoft, "____"+esc(caption)+"____"))
		b.WriteString(el("top:890px;left:0;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:47px;color:#ffffff;line-height:1.4;white-space:pre-line;"+thumbShSoft, prices))

	case "canva2": // Trang 2 — nền full + dải 3 ảnh giữa, Yeseva trắng
		b.WriteString(thumbImg(thumbPhoto(photos, 0), "top:0;left:0;width:1080px;height:1920px"))
		b.WriteString(thumbVeil("rgba(28,14,7,.45)"))
		b.WriteString(thumbImg(thumbPhoto(photos, 1), "top:745px;left:0;width:232px;height:555px"))
		b.WriteString(thumbImg(thumbPhoto(photos, 2), "top:745px;left:262px;width:556px;height:555px"))
		b.WriteString(thumbImg(thumbPhoto(photos, 3), "top:745px;left:848px;width:232px;height:555px"))
		b.WriteString(el("top:219px;left:0;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:104px;letter-spacing:1.5px;color:#ffffff;"+thumbShSoft, fitSpan(1000, title)))
		b.WriteString(el("top:347px;left:0;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:47px;color:#ffffff;"+thumbShSoft, fitSpan(1010, addr)))
		// Khối giá kẹp giữa địa chỉ và dải ảnh (745): 65px/dòng chỉ đủ 4 dòng
		// trong 268px — 5 dòng API phải nén, không thì dòng cuối đè dải ảnh.
		lh, fs := thumbFitLines(268, 65, 47, thumbLineCount(prices))
		b.WriteString(el(fmt.Sprintf("top:477px;left:0;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:%dpx;color:#ffffff;line-height:%dpx;white-space:pre-line;", fs, lh)+thumbShSoft, prices))
		if amen != "" {
			b.WriteString(el("top:1445px;left:-6px;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:46px;letter-spacing:.5px;color:#ffffff;"+thumbShSoft, fitSpan(1010, amen)))
		}

	case "canva3": // Trang 3 — lưới 2×2, chữ kem Comic + "homestay" viết tay + sao
		b.WriteString(thumbImg(thumbPhoto(photos, 0), "top:0;left:0;width:538px;height:958px"))
		b.WriteString(thumbImg(thumbPhoto(photos, 1), "top:0;left:542px;width:538px;height:958px"))
		b.WriteString(thumbImg(thumbPhoto(photos, 2), "top:962px;left:0;width:538px;height:958px"))
		b.WriteString(thumbImg(thumbPhoto(photos, 3), "top:962px;left:542px;width:538px;height:958px"))
		b.WriteString(thumbVeil("rgba(0,0,0,.08)"))
		b.WriteString(el("top:654px;left:-6px;width:1080px;text-align:center;font-family:'Pacifico',cursive;font-size:104px;color:#ffffe9;transform:rotate(-5deg);"+thumbShCream, "homestay"))
		b.WriteString(el("top:831px;left:12px;width:1080px;text-align:center;font-family:'Comic Neue','Baloo 2',cursive;font-weight:700;font-size:88px;color:#ffffe9;"+thumbShCream,
			`<span style="font-size:112px;line-height:1;color:#ffffff;vertical-align:6px">✦</span>`+fitSpan(760, title)+`<span style="font-size:103px;line-height:1;vertical-align:2px">✶</span>`))
		b.WriteString(el("top:937px;left:0;width:1080px;text-align:center;font-family:'Comic Neue','Baloo 2',cursive;font-weight:700;font-size:41px;color:#ffffe9;"+thumbShCream,
			`<span style="text-shadow:none">📍</span>`+fitSpan(960, addr)))
		b.WriteString(el("top:1004px;left:0;width:1080px;text-align:center;font-family:'Comic Neue','Baloo 2',cursive;font-weight:700;font-size:51px;color:#ffffe9;line-height:1.32;white-space:pre-line;"+thumbShCream, prices))

	case "canva4": // Trang 4 — nền full, "Homestay" + MÃ PHÒNG siêu to + 2 panel mờ
		b.WriteString(thumbImg(thumbPhoto(photos, 0), "top:0;left:0;width:1080px;height:1920px"))
		b.WriteString(thumbVeil("rgba(0,0,0,.12)"))
		b.WriteString(el("top:375px;left:55px;font-family:'Yeseva One',serif;font-size:66px;color:#ffffff;"+thumbShSoft, "Homestay"))
		big := title
		if ws := strings.Fields(strings.TrimSpace(cfg.Title)); len(ws) > 1 {
			// Mã phòng để phóng to = từ ĐẦU TIÊN có chữ số, ≥2 ký tự ("Amoureux
			// NK605" → "NK605", "p602 - phòng có ban công" → "p602", "502 - 1
			// NGỦ" → "502"); lấy từ cuối thì nick mô tả ra chữ vô nghĩa ("công",
			// "NGỦ"). Không từ nào có số → từ cuối như cũ ("CARTOON").
			big = esc(ws[len(ws)-1])
			for _, w := range ws {
				if len([]rune(w)) >= 2 && strings.ContainsAny(w, "0123456789") {
					big = esc(w)
					break
				}
			}
		}
		b.WriteString(el("top:429px;left:0;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:215px;color:#ffffff;"+thumbShHero, fitSpan(960, big)))
		b.WriteString(el("top:650px;left:0;width:1080px;text-align:center;font-family:'Poppins',sans-serif;font-weight:400;font-size:34.5px;color:#ffffff;"+thumbShSoft,
			`<span style="text-shadow:none">📍</span>`+fitSpan(960, addr)))
		if amen != "" {
			b.WriteString(el("top:722px;left:113px;width:838px;text-align:center;font-family:'Poppins',sans-serif;font-weight:400;font-size:41px;color:#ffffff;line-height:56px;",
				`<span style="display:inline-block;background:rgba(15,17,22,.28);border:5px solid rgba(255,255,255,.7);border-radius:28px;padding:15px 30px;max-width:838px">`+amen+`</span>`))
		}
		b.WriteString(el("bottom:140px;left:65px;font-family:'Poppins',sans-serif;font-weight:400;font-size:41px;color:#ffffff;line-height:55px;",
			`<span style="display:inline-block;background:rgba(15,17,22,.32);border:5px solid rgba(255,255,255,.7);border-radius:28px;padding:26px 60px 26px 35px;text-align:left;white-space:pre-line">`+prices+`</span>`))

	case "canva5": // Trang 5 — nền full, "Homestay" brush kem + Francois One
		b.WriteString(thumbImg(thumbPhoto(photos, 0), "top:0;left:0;width:1080px;height:1920px"))
		b.WriteString(thumbVeil("rgba(0,0,0,.08)"))
		b.WriteString(el("top:278px;left:-20px;width:1080px;text-align:center;font-family:'Kaushan Script',cursive;font-size:247px;letter-spacing:-9.5px;color:#ffffe9;"+thumbShCream, "Homestay"))
		b.WriteString(el("top:613px;left:0;width:1080px;text-align:center;font-family:'Francois One','Baloo 2',sans-serif;font-size:80px;color:#ffffe9;"+thumbShCream, fitSpan(940, title)))
		b.WriteString(el("top:717px;left:0;width:1080px;text-align:center;font-family:'Poppins',sans-serif;font-weight:500;font-size:41px;color:#ffffe9;"+thumbShCream, fitSpan(980, addr)))
		b.WriteString(el("top:791px;left:-8px;width:1080px;text-align:center;font-family:'Poppins',sans-serif;font-weight:500;font-size:40px;color:#ffffe9;line-height:1.39;white-space:pre-line;"+thumbShCream, prices))

	case "canva6": // Trang 6 — 2 ảnh dọc, title Anton kem + 2 panel mờ sáng
		b.WriteString(thumbImg(thumbPhoto(photos, 0), "top:0;left:0;width:1080px;height:1288px"))
		b.WriteString(thumbImg(thumbPhoto(photos, 1), "top:1292px;left:0;width:1080px;height:628px"))
		b.WriteString(thumbVeil("rgba(0,0,0,.10)"))
		b.WriteString(el("top:522px;left:370px;width:710px;text-align:center;font-family:'Poppins',sans-serif;font-weight:500;font-size:34px;color:#ffffff;"+thumbShSoft,
			`<span style="text-shadow:none">📍</span>`+fitSpan(660, addr)))
		b.WriteString(el("top:552px;left:0;width:1080px;text-align:center;font-family:'Anton',sans-serif;font-size:130px;color:#ffffe9;letter-spacing:2px;"+thumbShCream, fitSpan(980, title)))
		if amen != "" {
			b.WriteString(el("top:736px;left:95px;width:880px;text-align:center;font-family:'Poppins',sans-serif;font-weight:500;font-size:40px;color:#ffffff;line-height:1.42;",
				`<span style="display:inline-block;background:rgba(255,255,255,.16);border:1.5px solid rgba(255,255,255,.45);border-radius:28px;padding:16px 63px;max-width:880px">`+amen+`</span>`))
		}
		// Pill giá phải lọt trên mép ảnh dưới (1292): ≤5 dòng vừa nguyên bản
		// (1.39em ≈ 56px/dòng + padding 52); ≥6 dòng nén như canva2.
		priceCSS := "font-size:40px;color:#ffffff;line-height:1.39;"
		if n := thumbLineCount(prices); n*56 > 315 {
			lh6, fs6 := thumbFitLines(315, 56, 40, n)
			priceCSS = fmt.Sprintf("font-size:%dpx;color:#ffffff;line-height:%dpx;", fs6, lh6)
		}
		b.WriteString(el("top:925px;left:0;width:1080px;text-align:center;font-family:'Poppins',sans-serif;font-weight:500;"+priceCSS,
			`<span style="display:inline-block;background:rgba(255,255,255,.16);border:1.5px solid rgba(255,255,255,.45);border-radius:28px;padding:27px 40px 25px;white-space:pre-line">`+prices+`</span>`))

	case "valentine": // List Homestay Valentine — lưới 2×2 (3:4), badge tỉnh, tiêu
		// đề Playfair "LIST HOMESTAY" + "Valentine" Parisienne viền trắng, 4 nhãn tem.
		b.WriteString(thumbImg(thumbPhoto(photos, 0), "top:0;left:0;width:540px;height:720px"))
		b.WriteString(thumbImg(thumbPhoto(photos, 1), "top:0;left:540px;width:540px;height:720px"))
		b.WriteString(thumbImg(thumbPhoto(photos, 2), "top:720px;left:0;width:540px;height:720px"))
		b.WriteString(thumbImg(thumbPhoto(photos, 3), "top:720px;left:540px;width:540px;height:720px"))
		// veil xuyên tâm cho chữ giữa nổi
		b.WriteString(`<div style="position:absolute;inset:0;background:radial-gradient(120% 70% at 50% 46%,rgba(0,0,0,0) 30%,rgba(0,0,0,.40) 100%)"></div>`)

		// 4 nhãn tem tiện ích (so le, nghiêng), font Be Vietnam Pro cho đúng dấu.
		tags := valentineTags(cfg.Amenities)
		tagBase := "position:absolute;display:inline-block;width:max-content;background:#e8617a;color:#ffffff;" +
			"font-family:'Be Vietnam Pro',sans-serif;font-weight:700;font-size:22px;line-height:1.15;" +
			"padding:5px 13px;border-radius:24px;border:2.5px solid #ffffff;box-shadow:0 6px 16px rgba(0,0,0,.45);white-space:nowrap;z-index:6;"
		b.WriteString(`<div style="` + tagBase + `top:31%;left:9%;transform:rotate(-6deg)">` + esc(tags[0]) + `</div>`)
		b.WriteString(`<div style="` + tagBase + `top:31%;right:8%;transform:rotate(5deg)">` + esc(tags[1]) + `</div>`)
		b.WriteString(`<div style="` + tagBase + `top:57%;left:9%;transform:rotate(9deg)">` + esc(tags[2]) + `</div>`)
		b.WriteString(`<div style="` + tagBase + `top:57%;right:5%;transform:rotate(-9deg)">` + esc(tags[3]) + `</div>`)

		// Khối chữ giữa: badge (nền trắng chữ hồng) + LIST HOMESTAY + Valentine.
		// 3 phần đều cho phép nhập tay (ValBadge/ValTitle/ValScript); rỗng = mặc định.
		badge := valentineProvince(cfg.Address)
		if v := strings.TrimSpace(cfg.ValBadge); v != "" {
			badge = strings.ToUpper(v)
		}
		titleLine := "LIST HOMESTAY"
		if v := strings.TrimSpace(cfg.ValTitle); v != "" {
			titleLine = v
		}
		scriptLine := "Valentine"
		if v := strings.TrimSpace(cfg.ValScript); v != "" {
			scriptLine = v
		}
		var c strings.Builder
		c.WriteString(`<div style="position:absolute;left:0;right:0;top:46%;transform:translateY(-50%);text-align:center;z-index:5">`)
		c.WriteString(`<div style="display:inline-block;background:#ffffff;color:#e24b6e;font-family:'Be Vietnam Pro',sans-serif;font-weight:700;letter-spacing:2.5px;font-size:40px;line-height:1.05;padding:5px 26px;border-radius:40px;box-shadow:0 6px 20px rgba(0,0,0,.4);margin-bottom:14px">` + esc(badge) + `</div>`)
		c.WriteString(`<div style="font-family:'Playfair Display',serif;color:#ffffff;font-size:62px;letter-spacing:8px;font-weight:400;display:block;transform:scaleY(1.5);transform-origin:center;text-shadow:0 2px 10px rgba(0,0,0,.6);margin:20px 0 -38px;position:relative;z-index:5">` + esc(titleLine) + `</div>`)
		// "Valentine" dạng HTML text (KHÔNG dùng SVG): web-font Parisienne trong SVG
		// hay bị Chrome headless chụp trước khi nạp xong → rớt về Comic Sans. HTML
		// text nạp font ổn định như các template canva. Viền trắng: text-stroke +
		// paint-order (stroke nằm dưới fill → viền lộ ra ngoài nét chữ).
		c.WriteString(`<div style="font-family:'ParisienneVB','Parisienne',cursive;font-size:128px;line-height:1;color:#e24b6e;-webkit-text-stroke:7px #ffffff;paint-order:stroke fill;text-shadow:0 3px 7px rgba(0,0,0,.45);display:inline-block;transform:scaleX(.92);transform-origin:center;position:relative;z-index:6">` + esc(scriptLine) + `</div>`)
		if p := strings.TrimSpace(cfg.ValPrice); p != "" {
			c.WriteString(`<div style="margin-top:16px"><span style="display:inline-block;background:#e24b6e;color:#ffffff;font-family:'Be Vietnam Pro',sans-serif;font-weight:700;font-size:30px;letter-spacing:.5px;padding:7px 24px;border-radius:24px;border:2.5px solid #ffffff;box-shadow:0 6px 16px rgba(0,0,0,.4)">Qua đêm chỉ từ ` + esc(p) + `</span></div>`)
		}
		c.WriteString(`</div>`)
		b.WriteString(c.String())

	default:
		return ""
	}
	return b.String()
}

// valentineProvince rút TÊN TỈNH/THÀNH (đoạn cuối địa chỉ) cho badge template
// "valentine", viết HOA. Rỗng → mặc định "HÀ NỘI".
func valentineProvince(addr string) string {
	var parts []string
	for _, p := range strings.Split(cleanVideoAddress(addr), ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	n := len(parts)
	if n == 0 {
		return "HÀ NỘI"
	}
	prov := canonicalProvince(parts[n-1]) // tỉnh/thành có dấu chuẩn
	if n == 1 {
		return strings.ToUpper(prov)
	}
	// badge = Quận/Huyện + Tỉnh (VD "QUẬN 7 - HỒ CHÍ MINH", "ĐỐNG ĐA - HÀ NỘI")
	district := strings.TrimSpace(parts[n-2])
	return strings.ToUpper(district + " - " + prov)
}

// valentineTags trả đúng 4 nhãn tem: lấy tiện ích thật, thiếu thì bù mặc định.
func valentineTags(amenities []string) [4]string {
	def := [4]string{"Giường Kingsize", "Máy chiếu 4K", "Bếp đủ đồ", "Checkin không lễ tân"}
	out := def
	i := 0
	for _, a := range amenities {
		if a = strings.TrimSpace(a); a != "" && i < 4 {
			out[i] = a
			i++
		}
	}
	return out
}

// Bóng chữ thumbnail: glow rất nhẹ + drop mềm cho tách nền (đã chốt qua vòng lặp).
const thumbShSoft = "text-shadow:0 0 10px rgba(255,255,255,.35),0 2px 8px rgba(0,0,0,.35);"
const thumbShHero = "text-shadow:0 0 14px rgba(255,255,255,.35),0 3px 12px rgba(0,0,0,.35);"
const thumbShCream = "text-shadow:0 0 8px rgba(255,255,233,.3),0 2px 8px rgba(0,0,0,.3);"

// renderChromeThumbnail render template canvaN ra JPEG 1080×1920 (cỡ gốc design).
func renderChromeThumbnail(cfg ThumbnailConfig, photos []string) ([]byte, error) {
	name := strings.ToLower(strings.TrimSpace(cfg.Template))
	body := chromeThumbBody(name, cfg, photos)
	if body == "" {
		return nil, fmt.Errorf("thumbnail chrome: template %q không hỗ trợ", name)
	}
	stageW, stageH := thumbStageWH(name)
	extraFontCSS := ""
	if name == "valentine" {
		extraFontCSS = valentineFontCSS()
	}
	page := `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`*{margin:0;padding:0;box-sizing:border-box}html,body{background:#000}` +
		fontFaceCSS() + extraFontCSS +
		fmt.Sprintf(`.stage{width:%dpx;height:%dpx;position:relative;overflow:hidden;background:#111}`, stageW, stageH) +
		`img{display:block}` +
		`</style></head><body><div class="stage">` + body + `</div>` + autofitScript + `</body></html>`

	tmpDir, err := os.MkdirTemp("", "thumbchrome")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	htmlPath := filepath.Join(tmpDir, "thumb.html")
	if err := os.WriteFile(htmlPath, []byte(page), 0o644); err != nil {
		return nil, err
	}
	outPath := filepath.Join(tmpDir, "thumb.png")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	args := []string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--force-device-scale-factor=1", "--default-background-color=ff000000",
		"--run-all-compositor-stages-before-draw", "--virtual-time-budget=6000",
		fmt.Sprintf("--window-size=%d,%d", stageW, stageH), "--user-data-dir=" + filepath.Join(tmpDir, "profile"),
		"--screenshot=" + outPath, "file://" + htmlPath,
	}
	if out, err := exec.CommandContext(ctx, chromeBin(), args...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("thumbnail chrome: %v: %s", err, string(out))
	}
	img, err := imaging.Open(outPath)
	if err != nil {
		return nil, fmt.Errorf("thumbnail chrome: đọc screenshot: %v", err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, fmt.Errorf("thumbnail chrome: encode: %v", err)
	}
	return buf.Bytes(), nil
}
