package main

// Overlay video render bằng HEADLESS CHROME thay vì Go-canvas (gg). Mỗi template
// dựng đúng HTML/CSS đã chốt trong mockup_templates.html (vị trí px tuyệt đối,
// font + cỡ + màu + viền + bóng y hệt), chỉ THAY nội dung chữ bằng data thật.
// Chrome screenshot nền trong suốt 1080×1920 → composite.png → ffmpeg overlay 0:0
// như mọi overlay khác (giữ nguyên đường ống main.go).
//
// Vì sao Chrome: -webkit-text-stroke (viền Canva), box-shadow/border pill, emoji
// màu (🐟🌿📍), text-shadow đổ bóng cứng maroon, SVG textPath cong (editorial)
// render giống Canva hơn hẳn cách vẽ tay bằng gg. Data layer (video_data.go) giữ
// nguyên: giá đã nén, địa chỉ đã dọn được nhồi vào HTML.

import (
	"context"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// chromeBin trả về binary chrome (cho override qua env trong CI/headless).
func chromeBin() string {
	for _, k := range []string{"CHROME_BIN", "GOOGLE_CHROME_BIN"} {
		if b := strings.TrimSpace(os.Getenv(k)); b != "" {
			return b
		}
	}
	return "google-chrome"
}

func esc(s string) string { return html.EscapeString(s) }

// escLines escape từng dòng rồi nối bằng "\n" (div white-space:pre-line tự xuống dòng).
func escLines(lines []string) string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, html.EscapeString(l))
	}
	return strings.Join(out, "\n")
}

// priceFlatLines làm phẳng priceGroups thành list dòng giá không rỗng.
func priceFlatLines(cfg Config) []string {
	var out []string
	for _, l := range priceGroups(cfg.Prices) {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// withFish thêm 🐟 cuối mỗi dòng giá (chillgreen/marquee — "cá" = đơn vị tiền trong Canva).
func withFish(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l + "🐟"
	}
	return out
}

// fontFaceCSS map mỗi family → file ttf cục bộ (file://) để render OFFLINE và khớp
// đúng font mockup. Poppins tải về assets/fonts; còn lại đã có sẵn.
func fontFaceCSS() string {
	dir, _ := filepath.Abs(filepath.Join(assetsDir(), "fonts"))
	ff := func(family, file, weight, style string) string {
		return fmt.Sprintf("@font-face{font-family:'%s';src:url('file://%s/%s') format('truetype');font-weight:%s;font-style:%s;font-display:block}",
			family, dir, file, weight, style)
	}
	return strings.Join([]string{
		ff("Baloo 2", "Baloo2-Bold.ttf", "700", "normal"),
		ff("Baloo 2", "Baloo2-ExtraBold.ttf", "800", "normal"),
		ff("Yeseva One", "YesevaOne-Regular.ttf", "400", "normal"),
		ff("Dancing Script", "DancingScript-Variable.ttf", "600", "normal"),
		ff("Dancing Script", "DancingScript-Variable.ttf", "700", "normal"),
		ff("Poppins", "Poppins-Light.ttf", "300", "normal"),
		ff("Poppins", "Poppins-Regular.ttf", "400", "normal"),
		ff("Poppins", "Poppins-SemiBold.ttf", "600", "normal"),
		ff("Poppins", "Poppins-Bold.ttf", "700", "normal"),
		ff("Poppins", "Poppins-BoldItalic.ttf", "700", "italic"),
		ff("Paytone One", "PaytoneOne-Regular.ttf", "400", "normal"),
		ff("Montserrat", "Montserrat-Regular.ttf", "400", "normal"),
		ff("Montserrat", "Montserrat-SemiBold.ttf", "600", "normal"),
		ff("Montserrat", "Montserrat-Bold.ttf", "700", "normal"),
		ff("Sacramento", "Sacramento-Regular.ttf", "400", "normal"),
		ff("Prata", "Prata-Regular.ttf", "400", "normal"),
		// Cần cả bản Book (400): SVG "Standard" (editorial) không set font-weight
		// → nếu chỉ khai 700, Chrome lấy file Bold → đậm hơn trang mockup (render
		// bằng DejaVu Serif hệ thống, weight 400).
		ff("DejaVu Serif", "DejaVuSerif.ttf", "400", "normal"),
		ff("DejaVu Serif", "DejaVuSerif-Bold.ttf", "700", "normal"),
		// Font cho thumbnail canva1..6 (chrome_thumbnails.go) — thay thế font
		// Canva trả phí, xem comment đầu file đó.
		ff("Comic Neue", "ComicNeue-Regular.ttf", "400", "normal"),
		ff("Comic Neue", "ComicNeue-Bold.ttf", "700", "normal"),
		ff("Pacifico", "Pacifico-Regular.ttf", "400", "normal"),
		ff("Anton", "Anton-Regular.ttf", "400", "normal"),
		ff("Kaushan Script", "KaushanScript-Regular.ttf", "400", "normal"),
		ff("Francois One", "FrancoisOne-Regular.ttf", "400", "normal"),
	}, "")
}

// veilCSS = bản CSS của drawVideoVeil(Go): darken đều .345 + gradient đậm đáy (.55)
// + đậm nhẹ đỉnh (.38) cho legibility — bake vào overlay nên video thật cũng tối nền.
const veilCSS = `.veil{position:absolute;inset:0;background:` +
	`linear-gradient(to bottom,rgba(6,4,3,.38) 0%,rgba(6,4,3,0) 26%,rgba(6,4,3,0) 40%,rgba(6,4,3,.55) 100%),` +
	`rgba(6,4,3,.345)}`

// autofitScript co chữ tiêu đề (.fit, đo bề rộng span) cho vừa vùng an toàn —
// giống renderElFitWidth của Go. PHẢI chờ document.fonts.ready: nếu đo trước khi
// @font-face load, dùng metric font fallback → co sai → font thật load lại to hơn
// → TRÀN (FOUT race). Pass 2 (ntgroom): dời pill "Room" BÁM ĐUÔI tên thật —
// left = mép-phải-tên − 290 (calibrate mockup: "NTG402"@170px rộng 659.6 → pill
// left 53+659.6−290 ≈ 423 = đúng mockup). Tên ngắn ("AC21") pill bám tên thay vì
// đứng cố định 423 lơ lửng giữa khung (hết "bị lệch").
const autofitScript = `<script>
document.fonts.ready.then(function(){
 for(const el of document.querySelectorAll('.fit')){
  const max=parseFloat(el.dataset.max);el.style.whiteSpace='nowrap';
  let s=parseFloat(getComputedStyle(el).fontSize);
  while(el.getBoundingClientRect().width>max&&s>14){s-=2;el.style.fontSize=s+'px'}}
 const nt=document.getElementById('ntgT'),np=document.getElementById('ntgP');
 if(nt&&np){
  const st=document.querySelector('.stage').getBoundingClientRect();
  const k=st.width/1080||1;
  const tr=(nt.getBoundingClientRect().right-st.left)/k;
  let x=Math.round(tr)-290;if(x<53)x=53;
  np.style.left=x+'px'}
});
</script>`

// el dựng 1 div absolute. css = phần style (không gồm position:absolute).
func el(css, inner string) string {
	return `<div style="position:absolute;` + css + `">` + inner + `</div>`
}

// chromePage bọc body vào trang 1080×1920 (scale nếu video khác cỡ), kèm veil + font + autofit.
func chromePage(cfg Config, body string) string {
	sx := float64(cfg.Width) / 1080.0
	sy := float64(cfg.Height) / 1920.0
	transform := ""
	if sx != 1.0 || sy != 1.0 {
		transform = fmt.Sprintf("transform:scale(%.6f,%.6f);transform-origin:top left;", sx, sy)
	}
	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`*{margin:0;padding:0;box-sizing:border-box}html,body{background:transparent}` +
		fontFaceCSS() +
		`.stage{width:1080px;height:1920px;position:relative;overflow:hidden;background:transparent;` + transform + `}` +
		veilCSS +
		`</style></head><body><div class="stage"><div class="veil"></div>` +
		body + autofitScript +
		`</body></html>`
}

// wmBlock vẽ WATERMARK (chỉ khi user nhập) neo ở ĐÁY vùng an toàn TikTok
// (top:1526px = 0.795H; đáy an toàn = 0.82H×1920 ≈ 1574px) để đọc như footer,
// không lơ lửng giữa khung. KHÔNG vẽ listing_id — mã thô ("lsv3ix6oij") không có
// trong mockup, trước đây đè lên pill địa chỉ/title gây rối. Editorial dùng
// watermark làm brand đỉnh nên bỏ wm ở đây.
func wmBlock(name string, cfg Config) string {
	if wm := strings.TrimSpace(cfg.Watermark); wm != "" && name != "editorial" {
		return el("top:1526px;left:0;width:1080px;text-align:center;font-family:'Montserrat',sans-serif;font-size:38px;color:#fff;font-weight:400;text-shadow:0 2px 4px rgba(0,0,0,.6)", esc(wm))
	}
	return ""
}

// ─── style fragment dùng chung (đúng mockup) ──────────────────────────────────

const shSoft = "text-shadow:0 2px 6px rgba(0,0,0,.55),0 0 4px rgba(0,0,0,.4);"
const shHero = "text-shadow:0 4px 12px rgba(0,0,0,.55),0 0 8px rgba(0,0,0,.4);"

// Tagline vàng CỐ ĐỊNH của template chillgreen (Trang 4) — giữ nguyên mọi listing
// theo yêu cầu; tên phòng hiển thị riêng ở dòng hero phía trên.
const chillTagline1 = "Homestay chữa lành siêu chill"
const chillTagline2 = "đầy đủ tiện nghi, máy chiếu Netflix"

// fitSpan bọc text vào span auto-co bề rộng (cho tiêu đề trong div center).
func fitSpan(max int, text string) string {
	return fmt.Sprintf(`<span class="fit" data-max="%d">%s</span>`, max, text)
}

// noOrphanTail thay dấu cách CUỐI bằng NBSP để khối wrap không đẩy một từ mồ
// côi xuống dòng ("…, Hà Nội" wrap giữ nguyên cụm "Hà Nội").
func noOrphanTail(s string) string {
	if i := strings.LastIndex(s, " "); i > 0 {
		return s[:i] + "\u00A0" + s[i+1:]
	}
	return s
}

// chromeTemplateBody dựng phần body HTML theo template, nhồi data thật từ cfg.
func chromeTemplateBody(name string, cfg Config) string {
	nick := esc(videoNickname(cfg))
	addr := esc(strings.TrimSpace(cfg.Address))
	district := esc(districtFromAddress(cfg.Address))
	amen := esc(strings.Join(trimNonEmpty(cfg.Amenities), ", "))
	flat := priceFlatLines(cfg)

	var b strings.Builder
	switch name {

	case "goldserif": // Trang 1
		b.WriteString(el("top:398px;left:0px;width:1080px;text-align:center;font-family:'Baloo 2',cursive;font-size:40px;color:#fffefe;font-weight:700;line-height:1.18;paint-order:stroke fill;-webkit-text-stroke:5.5px #000;"+shSoft, addr))
		b.WriteString(el("top:452px;left:0px;width:1080px;text-align:center;font-family:'Baloo 2',cursive;font-size:95px;color:#fffc00;font-weight:800;line-height:1.18;paint-order:stroke fill;-webkit-text-stroke:10px #000;"+shHero, fitSpan(980, nick)))
		// Bảng giá: nhóm header vàng + dòng dữ liệu trắng, flow dọc canh giữa từ top 600.
		var rows strings.Builder
		var buf []string
		flush := func() {
			if len(buf) > 0 {
				rows.WriteString(`<div style="text-align:left;font-family:'Baloo 2',cursive;font-size:42px;color:#fffefe;font-weight:700;line-height:1.45;white-space:pre-line;paint-order:stroke fill;-webkit-text-stroke:8px #000;` + shSoft + `">` + escLines(buf) + `</div>`)
				buf = nil
			}
		}
		for _, ln := range goldSerifGroups(flat) {
			if strings.ContainsAny(ln, "0123456789") {
				buf = append(buf, ln)
			} else {
				flush()
				rows.WriteString(`<div style="font-family:'Baloo 2',cursive;font-size:47px;color:#fffc00;font-weight:800;line-height:1.18;paint-order:stroke fill;-webkit-text-stroke:9px #000;` + shSoft + `">` + esc(ln) + `</div>`)
			}
		}
		flush()
		b.WriteString(`<div style="position:absolute;top:600px;left:0;width:1080px;display:flex;flex-direction:column;align-items:center;gap:14px">` + rows.String() + `</div>`)

	case "creampill": // Trang 2
		top := escLines(pickTwoPriceLines(flat))
		topJoined := strings.ReplaceAll(top, "\n", " - ")
		b.WriteString(el("top:396px;left:0px;width:1080px;text-align:center;font-family:'Baloo 2',cursive;font-size:58px;color:#f9e9af;font-weight:700;line-height:1.18;paint-order:stroke fill;-webkit-text-stroke:9.5px #000;"+shSoft, topJoined))
		b.WriteString(el("top:503px;left:0px;width:1080px;text-align:center;font-family:'Baloo 2',cursive;font-size:109px;color:#ff4004;font-weight:800;line-height:1.0;",
			`<span class="fit" data-max="900" style="display:inline-block;background:#f9e9af;padding:33px 8px;border-radius:46px;box-shadow:0 4px 14px rgba(0,0,0,.28)">`+nick+`</span>`))
		b.WriteString(el("top:719px;left:0px;width:1080px;text-align:center;font-family:'Baloo 2',cursive;font-size:51px;color:#f9e9af;font-weight:700;line-height:1.18;paint-order:stroke fill;-webkit-text-stroke:12px #000;"+shSoft, addr))

	case "staycation": // Trang 3
		b.WriteString(el("top:550px;left:96px;width:430px;text-align:left;font-family:'Dancing Script',cursive;font-size:66px;color:#fffefe;font-weight:600;line-height:1.18;"+shSoft, "Staycation"))
		if district != "" {
			b.WriteString(el("top:550px;left:600px;width:374px;text-align:right;font-family:'Dancing Script',cursive;font-size:66px;color:#fffefe;font-weight:600;line-height:1.18;"+shSoft, district))
		}
		b.WriteString(el("top:650px;left:0px;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:89px;color:#fffefe;font-weight:400;line-height:1.18;"+shSoft, fitSpan(1000, nick)))
		if amen != "" {
			b.WriteString(el("top:814px;left:0px;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:43px;color:#fffefe;font-weight:400;line-height:1.18;"+shSoft, amen))
		}
		b.WriteString(el("top:916px;left:0px;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:43px;color:#fffefe;font-weight:400;line-height:1.40;white-space:pre-line;"+shSoft, escLines(flat)))

	case "chillgreen": // Trang 4
		// TÊN PHÒNG chữ SIÊU TO trên cùng (hero) — font khớp template (Poppins ExtraBold),
		// viền đen dày để nổi trên nền ảnh sáng.
		b.WriteString(el("top:248px;left:0px;width:1080px;text-align:center;font-family:'Poppins',sans-serif;font-size:140px;color:#ffffff;font-weight:800;line-height:1.02;paint-order:stroke fill;-webkit-text-stroke:9px #000;"+shHero, fitSpan(1000, nick)))
		b.WriteString(el("top:451px;left:0px;width:1080px;text-align:center;font-family:'Poppins',sans-serif;font-size:40px;color:#ffffff;font-weight:700;line-height:1.18;"+shSoft, esc(districtProvinceLine(cfg.Address))))
		// Dòng tagline VÀNG CỐ ĐỊNH (giữ nguyên theo yêu cầu — không thay bằng data).
		headline := esc(chillTagline1) + "\n" + esc(chillTagline2)
		b.WriteString(el("top:500px;left:0px;width:1080px;text-align:center;font-family:'Poppins',sans-serif;font-size:46px;color:#ffc700;font-weight:700;font-style:italic;line-height:1.33;white-space:pre-line;text-shadow:5px 5px 0 #812c2c;",
			`<span style="text-shadow:none">🌿</span>`+headline+`<span style="text-shadow:none">🌿</span>`))
		fish := withFish(flat)
		blk := func(top int, lines []string) {
			if len(lines) == 0 {
				return
			}
			b.WriteString(el(fmt.Sprintf("top:%dpx;left:322px;width:600px;", top)+"text-align:left;font-family:'Poppins',sans-serif;font-size:40px;color:#ffffff;font-weight:700;line-height:1.37;white-space:pre-line;"+shSoft, escLines(lines)))
		}
		if len(fish) <= 2 {
			blk(654, fish)
		} else {
			blk(654, fish[:2])
			blk(778, fish[2:])
		}

	case "marquee": // Trang 5 (headline cố định template)
		b.WriteString(el("top:480px;left:0px;width:326px;text-align:right;font-family:'Yeseva One',serif;font-size:50px;color:#f9e9af;font-weight:400;line-height:1.18;"+shSoft, "Summer"))
		b.WriteString(el("top:480px;left:755px;width:340px;text-align:left;font-family:'Yeseva One',serif;font-size:50px;color:#f9e9af;font-weight:400;line-height:1.18;"+shSoft, "Hà Nội"))
		b.WriteString(el("top:455px;left:0px;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:120px;color:#ffbf1f;font-weight:400;line-height:1.18;text-shadow:9px 11px 0 #812c2c;", "Date"))
		b.WriteString(el("top:590px;left:0px;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:120px;color:#ffbf1f;font-weight:400;line-height:1.18;text-shadow:9px 11px 0 #812c2c;", "Homestay"))
		if amen != "" {
			b.WriteString(el("top:738px;left:0px;width:1080px;text-align:center;font-family:'Yeseva One',serif;font-size:46px;color:#f9e9af;font-weight:400;line-height:1.18;"+shSoft, amen))
		}
		fish := withFish(flat)
		blk := func(top int, lines []string) {
			if len(lines) == 0 {
				return
			}
			b.WriteString(el(fmt.Sprintf("top:%dpx;left:358px;width:600px;", top)+"text-align:left;font-family:'Yeseva One',serif;font-size:36px;color:#ffffff;font-weight:400;line-height:1.42;white-space:pre-line;"+shSoft, escLines(lines)))
		}
		if len(fish) <= 2 {
			blk(815, fish)
		} else {
			blk(815, fish[:2])
			blk(939, fish[2:])
		}

	case "editorial": // Trang 6
		if wm := strings.TrimSpace(cfg.Watermark); wm != "" {
			b.WriteString(el("top:60px;left:0px;width:1080px;text-align:center;font-family:'Prata',serif;font-size:48px;color:#ffffff;font-weight:400;letter-spacing:2px;"+shSoft, esc(wm)))
		}
		// Chữ cung lớn CỐ ĐỊNH "Standard" y như mockup Canva Trang 6 (yêu cầu
		// 2026-07-03) — KHÔNG nhồi tên phòng (tên nhận diện qua intro title +
		// watermark + địa chỉ). Cỡ 188 cố định = mockup, khỏi autofit.
		b.WriteString(`<svg width="1080" height="700" viewBox="0 0 1080 700" style="position:absolute;top:0px;left:0px;overflow:visible"><defs><path id="archStd" d="M 97,440 A 2000,2000 0 0 1 983,440" fill="none"></path><filter id="sh" x="-20%" y="-20%" width="140%" height="140%"><feDropShadow dx="0" dy="2" stdDeviation="3" flood-color="#000" flood-opacity="0.5"></feDropShadow></filter></defs><text font-family="'DejaVu Serif','Noto Serif',Georgia,serif" font-size="188" fill="#ffffff" text-anchor="middle" letter-spacing="2" filter="url(#sh)"><textPath href="#archStd" startOffset="50%">Standard</textPath></text></svg>`)
		b.WriteString(`<svg width="700" height="340" viewBox="0 0 700 340" style="position:absolute;top:398px;left:534px;overflow:visible"><defs><path id="archRoom" d="M 30,170 A 1000,1000 0 0 1 470,170" fill="none"></path><filter id="glowRoom" x="-30%" y="-30%" width="160%" height="160%"><feGaussianBlur stdDeviation="1.6" result="b"></feGaussianBlur><feComponentTransfer in="b" result="bf"><feFuncA type="linear" slope="0.55"></feFuncA></feComponentTransfer><feMerge><feMergeNode in="bf"></feMergeNode><feMergeNode in="SourceGraphic"></feMergeNode></feMerge></filter></defs><text font-family="'Sacramento',cursive" font-size="132" fill="#ffffff" text-anchor="middle" letter-spacing="3" filter="url(#glowRoom)"><textPath href="#archRoom" startOffset="50%">Room</textPath></text></svg>`)
		// Địa chỉ NGẮN (ước lượng 1 dòng, Poppins 31 ≈ .55em/ký tự ≤700px) → CSS y hệt
		// mockup Trang 6 (top 500, width 780, size 31). Địa chỉ DÀI → cột hẹp 470,
		// top 520: wrap về cột trái, KHÔNG đè chữ "Room" (cung phải x≥534) và KHÔNG
		// đè panel giá (cột phải x≥500).
		if float64(utf8.RuneCountInString(addr)+1)*31*0.55 <= 700 {
			b.WriteString(el("top:500px;left:12px;width:780px;text-align:left;font-family:'Poppins',sans-serif;font-size:31px;color:#ffffff;font-weight:400;line-height:1.18;white-space:pre-line;text-shadow:0 1px 4px rgba(0,0,0,.5);",
				`<span style="text-shadow:none;-webkit-text-stroke:0">📍</span>`+addr))
		} else {
			b.WriteString(el("top:520px;left:12px;width:470px;text-align:left;font-family:'Poppins',sans-serif;font-size:30px;color:#ffffff;font-weight:400;line-height:1.25;white-space:pre-line;text-shadow:0 1px 4px rgba(0,0,0,.5);",
				`<span style="text-shadow:none;-webkit-text-stroke:0">📍</span>`+noOrphanTail(addr)))
		}
		b.WriteString(el("top:578px;left:500px;width:520px;text-align:left;font-family:'Poppins',sans-serif;font-size:38px;color:#ffffff;font-weight:400;line-height:1.5;white-space:pre-line;",
			`<span style="display:inline-block;border:4.5px solid rgba(255,255,255,.8);padding:30px 36px;border-radius:30px">`+escLines(flat)+`</span>`))

	case "amorex": // Trang 7 — pill kem VIỀN TRẮNG (giá + địa chỉ)
		// Bảng giá: Poppins Light (300) theo yêu cầu 2026-07-05 (trước đây 700 đậm).
		b.WriteString(el("top:116px;left:72px;width:560px;text-align:left;font-family:'Poppins',sans-serif;font-size:40px;color:#812c2c;font-weight:300;line-height:1.3;white-space:pre-line;",
			`<span style="display:inline-block;background:#f9e9af;border:6px solid #ffffff;padding:24px 34px;border-radius:24px">`+escLines(flat)+`</span>`))
		b.WriteString(el("top:1189px;left:0px;width:1080px;text-align:center;font-family:'Paytone One',sans-serif;font-size:172px;color:#ffffff;font-weight:400;line-height:1.18;"+shHero, fitSpan(1000, nick)))
		// max-width 1000: địa chỉ dài wrap TRONG pill, pill vẫn nổi giữa với lề
		// ≥40px mỗi bên (không full-bleed 1080 nhìn như bị cắt). noOrphanTail giữ
		// "Hà Nội" liền cụm, không rớt một từ mồ côi xuống dòng 2.
		b.WriteString(el("top:1421px;left:0px;width:1080px;text-align:center;font-family:'Poppins',sans-serif;font-size:34px;color:#812c2c;font-weight:700;line-height:1.18;",
			`<span style="display:inline-block;max-width:1000px;background:#f9e9af;border:6px solid #ffffff;padding:14px 30px;border-radius:40px">📍`+noOrphanTail(addr)+`</span>`))

	case "ntgroom": // Trang 8
		b.WriteString(el("top:110px;left:58px;width:560px;text-align:left;font-family:'Montserrat',sans-serif;font-size:41px;color:#ffffff;font-weight:400;line-height:1.45;white-space:pre-line;",
			`<span style="display:inline-block;background:rgba(129,44,44,.40);border:5px solid rgba(255,255,255,.55);padding:27px 36px;border-radius:50px">`+escLines(flat)+`</span>`))
		// Tên: max 880 (không phải 980) để pill "Room" bám đuôi (+98px nhô quá mép
		// phải tên) vẫn nằm trong ≤1030. Pill left mặc định 423 = mockup NTG402;
		// autofitScript (#ntgT/#ntgP) dời pill = mép-phải-tên − 290 → tên ngắn
		// ("AC21") pill ôm sát tên, tên dài pill trượt theo, hết lệch bố cục.
		b.WriteString(el("top:955px;left:53px;width:980px;text-align:left;font-family:'Yeseva One',serif;font-size:170px;color:#ffffff;font-weight:400;line-height:1.18;"+shHero,
			`<span class="fit" data-max="880" id="ntgT">`+nick+`</span>`))
		b.WriteString(`<div id="ntgP" style="position:absolute;top:1129px;left:423px;width:560px;text-align:left;font-family:'Yeseva One',serif;font-size:92px;color:#ffffff;font-weight:400;line-height:1.0;text-shadow:0 1px 2px rgba(0,0,0,.28);">` +
			`<span style="display:inline-block;background:linear-gradient(180deg,#f7ad2e,#ef9606);padding:10px 62px;border-radius:60px">Room</span></div>`)
		b.WriteString(el("top:1336px;left:0px;width:1080px;text-align:center;font-family:'Poppins',sans-serif;font-size:25px;color:#ffffff;font-weight:700;line-height:1.18;",
			`<span style="display:inline-block;background:#1f6b3a;border:6px solid #ffffff;padding:20px 104px;border-radius:40px">📍`+addr+`</span>`))

	default:
		return ""
	}

	b.WriteString(wmBlock(name, cfg))
	return b.String()
}

// renderChromeOverlay dựng HTML cho template rồi chụp transparent 1080×1920 →
// composite.png. Lỗi → caller rơi về đường gg cũ.
func renderChromeOverlay(cfg Config, tmpDir string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(cfg.Template))
	body := chromeTemplateBody(name, cfg)
	if body == "" {
		return "", fmt.Errorf("chrome overlay: template %q không hỗ trợ", name)
	}
	htmlPath := filepath.Join(tmpDir, "overlay.html")
	if err := os.WriteFile(htmlPath, []byte(chromePage(cfg, body)), 0o644); err != nil {
		return "", err
	}
	outPath := filepath.Join(tmpDir, "composite.png")
	userDir := filepath.Join(tmpDir, "chrome-profile")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	args := []string{
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--force-device-scale-factor=1", "--default-background-color=00000000",
		"--run-all-compositor-stages-before-draw", "--virtual-time-budget=5000",
		"--user-data-dir=" + userDir,
		"--screenshot=" + outPath,
		fmt.Sprintf("--window-size=%d,%d", cfg.Width, cfg.Height),
		"file://" + htmlPath,
	}
	out, err := exec.CommandContext(ctx, chromeBin(), args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("chrome render: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if st, e := os.Stat(outPath); e != nil || st.Size() == 0 {
		return "", fmt.Errorf("chrome render: không tạo được %s", outPath)
	}
	return outPath, nil
}
