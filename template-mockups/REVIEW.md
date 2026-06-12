# Đối chiếu mockup ↔ thiết kế gốc (12/06/2026)

So sánh 11 cặp bằng multi-agent, các điểm HIGH đã được kiểm chứng lại thủ công
trên ảnh gốc. Bỏ qua khác biệt ảnh nền (placeholder) và UI TikTok (tim/comment/
search bar — là chrome của app, không thuộc thiết kế).

## Lỗi xuyên suốt (gần như mọi template)

1. **Chữ quá nhỏ & nét quá mảnh** — gốc dùng cỡ chữ lớn hơn 25–80% và weight
   Bold/ExtraBold; mockup đang render nhỏ + regular nên "lép" hơn hẳn.
2. **Thiếu viền sticker** — các template kiểu TikTok (goldserif, chillgreen,
   marquee) đều có stroke đậm quanh chữ (đen/nâu/đỏ-cam); mockup chỉ có bóng mờ.
3. **Căn lề sai** — staycation và strip: gốc căn GIỮA toàn bộ, mockup căn trái.
4. **Sai font bản chất** ở 3 template: goldserif (gốc = sans bo tròn siêu đậm,
   không phải serif), amber (gốc = retro serif chân loe kiểu Recoleta), editorial
   (gốc = serif thanh lịch swash mượt, mockup render nhám như phấn).

## VIDEO

### 1. staycation
- [HIGH] Căn GIỮA title + tiện ích + giá (mockup đang căn trái).
- [HIGH] Dòng tiện ích: serif đậm cùng họ title (mockup đang sans), cỡ ~52% title.
- [HIGH] Khối giá: serif BOLD, cỡ to hơn ~30%, line-height 1.4–1.5.
- [MED] "Staycation"/"Cầu Giấy" cùng baseline, cùng cỡ; lề trái/phải ~9%.
- [LOW] 2 nhãn script to thêm 10–15%; title đậm hơn.

### 2. creampill
- [HIGH] Pill phải chiếm ~85–90% bề rộng; title to ~1.5×, weight ExtraBold (~800).
- [MED] Hạ cả cụm xuống ~7%H (tâm pill ~30%H); dòng giá + địa chỉ to thêm ~35%.
- [LOW] Màu title ~#E8503C (đỏ cam tươi hơn); pill bo capsule (~45% chiều cao);
  shadow chữ trắng đậm hơn.

### 3. goldserif  ⟵ sai bản chất nhiều nhất
- [HIGH] Title: sans bo tròn siêu đậm (Baloo/Nunito Black...), KHÔNG phải serif.
- [HIGH] Màu title: vàng chanh phẳng ~#FCF400 + viền nâu đen dày ~0.15em
  (bỏ stroke trắng hiện tại).
- [HIGH] Các dòng giá: trắng ExtraBold + viền đen ~0.12em (kiểu sticker), to ~135%.
- [MED] Header mục: #FFF34A + viền đen, to ~120%; các dòng trong mục sát lại
  (gap ~55–60px @1920H), cả khối hạ xuống ~5%H.
- [LOW] Địa chỉ to ~115% + viền mỏng.

### 4. editorial
- [HIGH] Font "Standard": serif swash mượt tương phản cao (Playfair đang render
  kiểu nhám/grunge — kiểm tra lại biến thể/axis của PlayfairDisplay-Variable).
- [HIGH] "Standard" to ~1.7–1.8× (trải ~92% bề rộng).
- [HIGH] Panel giá: gốc là kính tối mờ rgba(80,70,60,~0.35) + VIỀN TRẮNG 2–3px
  + chữ TRẮNG (mockup đang kem đặc + chữ nâu — ngược tông).
- [MED] "room" script to ~150–170%, kết thúc ~92% bề rộng, đè nhẹ baseline title;
  chữ panel to ~120–130%; brand 2 tầng (lagom/HOUSE).
- [GHI CHÚ] Viền trắng cụt góc phải trên của mockup là dính từ ảnh nền placeholder
  (crop refV4 dính mép panel gốc) — đổi vùng crop, không phải lỗi code.

### 5. marquee
- [HIGH] Headline to thêm ~50–55%: "Homestay" ~90% bề rộng, "Date" ~33%.
- [HIGH] Khối giá: cùng font headline, màu vàng #FFC83D + bóng/viền đỏ sẫm
  #8B1A12, to ~150% (mockup đang kem nhạt, lạc tông).
- [MED] Subtitle: serif đậm #C9E8EF (xanh nhạt) + bóng; khối giá hạ xuống ~50%H.
- [LOW] "Summer"/"Hà Nội" sáng hơn (#F2E3B8) + bóng; cá = cá hề sọc trắng-cam
  (đã có sẵn dạng vẽ tay — chỉnh sọc), to ~110% cỡ chữ.
- [BỎ QUA] Comment bubble + badge teal + cột tim/lưu = UI TikTok.

### 6. chillgreen  ⟵ màu chữ sai bản chất
- [HIGH] Headline: fill TRẮNG + stroke ĐỎ-CAM (~#E2492E) dày 6–8% cỡ chữ, italic
  đậm (mockup đang xanh lá — sai; xanh chỉ là màu của icon lá 🌿).
- [HIGH] 3 dòng khung giờ KHÔNG có giá riêng: vẽ 2 vạch dọc trắng ‖ bên phải gom
  3 dòng, một giá "299 🐟" duy nhất căn giữa dọc theo nhóm.
- [MED] Thêm viền nâu-đỏ sẫm mỏng cho địa chỉ + bảng giá.
- [LOW] Cá màu XANH (🐟) chứ không cam; lá 🌿 nhánh nhiều lá; địa chỉ sát title
  hơn (~2.5%H); giảm overlay tối nền xuống ≤15%.

## THUMBNAIL

### 1. cento
- [HIGH] 2 nhãn "14H–12H/689K" + "22H–10H/489K" phải nằm 2 GÓC ĐÁY (~92%H),
  không phải ngay dưới panel.
- [HIGH] Panel màu đỏ gạch/maroon ~#76241B (mockup đang nâu socola).
- [HIGH] Thiếu dòng giá trong panel: "💰 Giá 2 người: 299k/3h - 499k/7h - 489k/Đêm"
  cùng hàng với địa chỉ.
- [MED] Viền trắng ~2px quanh panel; kẹp giấy bạc đè mép phải panel; bỏ chip nền
  trắng của icon (emoji trần, gốc dùng 😈 trước tên); panel rộng ~86% + dẹt hơn
  (padding/line-height -25%); badge "3/19" phải có pill nền đen mờ ở SÁT góc
  (hết chồng chữ COMBO); tiện ích xếp lưới 3 CỘT thẳng hàng; nhãn 4 góc to ~140%.
- [LOW] Gạch cam ➖ giữa 2 mốc giờ; bỏ letter-spacing title.

### 2. amber
- [HIGH] Khối giá phải có panel nâu đặc ~#7B4A38 bo góc 14–16px (mockup gần như
  không panel).
- [HIGH] Font title: retro serif chân loe (Recoleta-style), không phải serif
  thanh mảnh.
- [MED] Title to ~140–160% (mỗi dòng ~12–14%H), dòng cuối kết thúc ~86%H; pill
  "Room" đặt BÊN PHẢI đè lên đuôi title (tâm ~80% bề rộng); thêm wordmark "lag"
  trắng cạnh badge 1/4.
- [LOW] "Room" italic + pill oval; font giá bo tròn; pin cách chữ 6–8px.

### 3. strip
- [HIGH] Căn GIỮA toàn bộ text (mockup căn trái).
- [HIGH] Dải ảnh full-bleed (tràn 2 mép), ảnh GIỮA to hơn ~30–40% và cao hơn,
  nhô trên/dưới so với 2 ảnh bên.
- [MED] Địa chỉ + tiện ích: serif đậm cùng họ title (đang sans); khối giá bold;
  title hạ xuống ~12%H, giãn cách lại các khối (giá line-height 1.5, dải ảnh
  bắt đầu ~38–40%H, tiện ích ~73%H).
- [LOW] Bỏ bo góc ảnh.

### 4. creamgrid
- [HIGH] Font thiếu glyph tiếng Việt → "Nguyên Khang, Câu Giây", "bôn tăm" —
  đổi font serif đỏ sang bản đủ dấu VN hoặc fontprep lại.
- [HIGH] Lưới ảnh cao hơn (đáy ~77%H, ô gần vuông); khối giá xuống vùng 84–93%H.
- [HIGH] Subtitle địa chỉ: serif đậm cùng họ title (~#BE3620), không phải sans tròn.
- [MED] Bỏ bo góc ảnh; title to ~25–30% (đạt ~85% bề rộng); tiện ích + giá bold.
- [LOW] Cả dòng giá 1 cỡ chữ; lưới sát subtitle hơn.

### 5. filmstrip
- [HIGH] BỎ scrim tối phủ dải giữa (gốc không phủ — chỉ shadow theo chữ).
- [HIGH] Địa chỉ: serif BOLD trắng kem UỐN CONG vòng cung (~10–15°), to ~185%
  (mockup đang sans italic thẳng).
- [MED] Cả khối chữ dịch LÊN ~8–10% (địa chỉ ~30%H, title vắt qua mép nối frame
  1–2); title Bold/ExtraBold; tagline serif đậm với chuỗi "___" sát baseline
  (không phải gạch ngang giữa); khối giá to ~35–40% + bold + cách tagline ~5%H.
