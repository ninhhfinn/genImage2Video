# 📘 Hướng dẫn đưa Web Gen Video lên mạng (để người khác dùng)

> Máy của bạn vẫn chạy backend. Người khác chỉ truy cập qua một **link**.
> Không cần nhớ gì cả — chỉ làm theo 3 bước dưới đây.

---

## ▶️ BƯỚC 1 — Bật web (mỗi lần mở máy)

Mở **Terminal**, dán đúng 2 dòng này rồi Enter:

```bash
cd /home/vietle/Work/image2video/genImage2Video
./start-web.sh
```

Chờ vài giây, màn hình hiện ra cái khung như vầy:

```
==================================================================
  🌐  WEB CỦA BẠN ĐÃ LIVE:

      https://xxxx-xxxx.trycloudflare.com
==================================================================
```

👉 **Cái link `https://...trycloudflare.com` đó chính là web của bạn.**

---

## 📤 BƯỚC 2 — Gửi link cho người khác

- Copy cái link trong khung.
- Gửi qua Zalo / Messenger / nhắn tin... cho người bạn muốn.
- Họ mở link trên điện thoại hoặc máy tính là dùng được ngay.

---

## 🛑 BƯỚC 3 — Tắt web khi xong

- Quay lại cửa sổ Terminal đang chạy.
- Nhấn **`Ctrl` + `C`**.
- Xong. Web tắt, không ai vào được nữa.

---

## ⚠️ 4 ĐIỀU QUAN TRỌNG PHẢI NHỚ

| | Điều cần nhớ |
|---|---|
| 1️⃣ | **Giữ cửa sổ Terminal mở.** Đóng nó lại là web tắt ngay. |
| 2️⃣ | **Không tắt máy tính.** Tắt máy là web tắt. |
| 3️⃣ | **Mỗi lần bật lại, link sẽ ĐỔI KHÁC.** Luôn copy link mới nhất trong khung — link cũ không còn dùng được. |
| 4️⃣ | **Chỉ gửi link cho người tin tưởng.** Đừng đăng công khai lên Facebook — ai có link cũng dùng máy bạn để tạo video được. |

---

## ❓ Khi gặp trục trặc

**Chạy `./start-web.sh` mà báo lỗi "command not found"?**
→ Bạn đang đứng sai thư mục. Gõ lại dòng `cd /home/vietle/...` ở Bước 1 trước.

**Không thấy link hiện ra?**
→ Đợi thêm 10 giây. Nếu vẫn không có, nhấn `Ctrl + C` rồi chạy lại `./start-web.sh`.

**Người khác mở link báo lỗi / không vào được?**
→ Kiểm tra Terminal trên máy bạn còn đang chạy không (chưa nhấn Ctrl+C, chưa tắt máy).

---

## 💡 Muốn link CỐ ĐỊNH (không đổi mỗi lần)?

Hiện tại link là loại **tạm thời, miễn phí** nên đổi mỗi lần bật.
Nếu muốn một link riêng **không bao giờ đổi** (ví dụ `video.tencuaban.com`):
cần một tài khoản Cloudflare miễn phí + một tên miền.
→ Nhờ Claude set up giúp khi cần.

---

## 🔧 Thông tin kỹ thuật (tham khảo, không cần đọc)

- **Backend:** Go, chạy ở `localhost:8080` (lệnh `./image2video --web` trong thư mục `backend/`).
- **Frontend:** React, build sẵn vào `backend/dist/` (lệnh `npm run build` trong `frontend/`).
- **Link public:** tạo bằng **Cloudflare Tunnel** (`cloudflared`), không cần mở port router, không lộ IP nhà.
- **Script bật nhanh:** `start-web.sh` (tự bật backend + tạo link).
- **Cập nhật giao diện sau khi sửa code:** chạy `npm run build` trong `frontend/` (backend tự phục vụ bản mới).
- **Log:** backend `/tmp/genvideo-backend.log` · tunnel `/tmp/genvideo-tunnel.log`.
