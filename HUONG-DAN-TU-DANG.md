# Hướng dẫn Tự đăng social (TikTok + Facebook)

Render video xong, app **tự gửi** video + thông tin listing tới một **webhook** trên
[Make.com](https://www.make.com) (miễn phí). Make sẽ đăng lên TikTok / Facebook giúp bạn.

```
App (render xong)  ──POST video + caption──▶  Webhook Make.com  ──▶  Facebook Page (công khai)
                                                                └──▶  TikTok (nháp → bấm Đăng)
```

App **không** nói chuyện trực tiếp với TikTok/Facebook — Make lo phần đó. Nhờ vậy
bạn né được toàn bộ chuyện đăng ký app developer + chờ duyệt.

---

## 1. Bật trong app

1. Chạy app như bình thường (xem `README.md`).
2. Ở cột **Cài đặt**, kéo xuống mục **📤 Tự đăng social** → bật lên.
3. Dán **link Webhook** lấy từ Make (xem phần 2 bên dưới).
4. Tick nền tảng muốn đăng: **TikTok** và/hoặc **Facebook**.
5. Render như bình thường. Thanh tiến trình sẽ hiện:
   - `Đang gửi lên social…`
   - rồi `Hoàn tất! 12.3 MB — ✅ đã gửi đăng (tiktok, facebook)`
   - hoặc `⚠️ gửi đăng lỗi: …` (gửi lỗi **không** làm hỏng video — vẫn có trong Lịch sử để tải tay).

> **Lưu ý caption:** caption tự ghép từ **tiêu đề + địa chỉ + giá + tiện nghi**, nhưng
> địa chỉ/giá/tiện nghi chỉ có khi bật **"Hiển thị thông tin listing"**. Tắt overlay thì
> caption chỉ có tiêu đề.

---

## 2. Dựng Make.com (làm 1 lần)

### 2.1 Tạo webhook
1. Tạo tài khoản free trên make.com → **Create a new scenario**.
2. Thêm module đầu tiên: **Webhooks → Custom webhook**.
3. Bấm **Add** → đặt tên (vd `img2video`) → **Save** → **copy** đường link hiện ra.
4. Dán link đó vào ô **Webhook** trong app.

### 2.2 Cho Make "học" dữ liệu
1. Trong Make, để cửa sổ webhook đang ở trạng thái **"Determine data structure / listening"**.
2. Sang app: bật Tự đăng, dán link, **render 1 video thật** → app gửi 1 request.
3. Make nhận được → tự nhận diện các field. Xong bước này Make biết các trường:
   `video` (file), `caption`, `platforms`, `meta`.

### 2.3 Thêm Router 2 nhánh
Thêm module **Router** ngay sau webhook → tạo 2 nhánh:

**Nhánh Facebook** (đăng công khai)
- Đặt **Filter** trên nhánh: `platforms` **contains** `facebook`.
- Module: **Facebook Pages → Upload a Video** (hoặc *Create a Post* có video).
- Connection: nối tài khoản Facebook → chọn **Page** cần đăng.
- Map: trường video ← `video` (file từ webhook); message/caption ← `caption`.

**Nhánh TikTok** (vào nháp)
- Đặt **Filter** trên nhánh: `platforms` **contains** `tiktok`.
- Module: **TikTok → Upload a video / Post**.
- Connection: nối tài khoản TikTok.
- Map: video ← `video`; caption/title ← `caption`.

### 2.4 Bật scenario
- Góc dưới trái: gạt **ON**.
- Lịch chạy: chọn **Immediately as data arrives** (webhook chạy ngay khi có dữ liệu).

Xong. Từ giờ cứ render là tự đăng.

---

## 3. Dữ liệu app gửi (cho ai muốn tự cấu hình / dùng n8n)

`POST <webhook_url>` dạng `multipart/form-data`:

| Field | Kiểu | Nội dung |
|---|---|---|
| `video` | file | File `.mp4` đã render |
| `caption` | text | Caption đã ghép sẵn (tiêu đề + 📍địa chỉ + 💰giá + ✨tiện nghi + Mã) |
| `platforms` | text | Danh sách cách nhau dấu phẩy, vd `tiktok,facebook` |
| `meta` | text (JSON) | Toàn bộ metadata: `title, caption, nickname, address, listing_id, prices[], amenities[], platforms[]` |

- Chỉ gửi khi: **bật Tự đăng** + có link webhook + tick ít nhất 1 nền tảng.
- Timeout phía app: **5 phút**. Webhook trả mã ngoài 2xx được coi là lỗi (báo lên UI, không chặn video).

---

## 4. Giới hạn TikTok (đọc kỹ)

TikTok **không cho** app chưa được audit đăng công khai tự động. Nên với TikTok, Make chỉ
đẩy video vào **nháp/inbox** — bạn mở app TikTok trên điện thoại bấm **Đăng** lần cuối.
Đây là luật của TikTok, **không cách nào lách** (free hay trả phí) nếu chưa qua audit.
Facebook / YouTube / Instagram thì auto công khai được.

---

## 5. Xử lý sự cố

| Hiện tượng | Cách xử lý |
|---|---|
| UI báo `gửi đăng lỗi: webhook trả mã 4xx/5xx` | Mở Make, xem scenario lỗi ở đâu (token social hết hạn? thiếu quyền Page?). |
| `gửi webhook: … timeout` hoặc treo lâu | Video quá to so với giới hạn webhook free của Make → xem phần 6. |
| Make không nhận được gì | Sai link webhook, hoặc scenario đang **OFF**, hoặc app chưa tick nền tảng nào. |
| Đăng được Facebook, TikTok im | Kiểm tra Filter nhánh TikTok (`platforms` contains `tiktok`) và connection TikTok. |

---

## 6. Nếu video quá to (cách B — qua Google Drive)

Webhook free của Make có giới hạn dung lượng nhận. Video 9:16 ~40s thường 5–20MB nên
**phần lớn ổn**. Nếu file to bị Make từ chối, hướng bền hơn là:

```
App → upload video lên Google Drive → gửi Make cái LINK → Make tải về rồi đăng
```

Cách này **chưa code** (hiện app gửi thẳng file — cách A). Khi cần, báo để bổ sung:
thêm bước upload Drive trong `backend/social.go` và gửi `video_url` thay cho file.

---

## Tóm tắt 30 giây
1. Make.com → tạo Custom webhook → copy link.
2. App → bật **📤 Tự đăng social** → dán link → tick TikTok/Facebook.
3. Make → Router 2 nhánh (filter theo `platforms`) → nối Facebook Page + TikTok.
4. Render → Facebook tự đăng, TikTok vào nháp (bấm Đăng tay).
