# Deploy & vận hành img2video (web Thuyết minh AI cho team)

Tool này biến ảnh homestay → video dọc TikTok có giọng thuyết minh AI. Chạy dạng
web nội bộ trên 1 VPS Linux; team truy cập qua link Cloudflare.

Kiến trúc: **local sửa code → push GitHub → server `update.sh` kéo về → chạy**.
Server **không cần Node** vì `frontend/dist` được build sẵn & commit ở local.

---

## A. Cài lần đầu trên VPS (1 lần duy nhất)

```bash
# SSH vào VPS rồi:
sudo apt-get install -y git
git clone https://github.com/ninhhfinn/genImage2Video.git /tmp/img2video-bootstrap
sudo bash /tmp/img2video-bootstrap/deploy/setup-server.sh
```

Script tự cài: ffmpeg, Chrome (deb, **không snap**), Go 1.25 (tarball), gdown
(venv), tạo user `img2video`, clone repo vào `/opt/img2video`, tạo
`/etc/img2video/secrets.env`, cài Claude Code (gói Max), dựng systemd service +
timer dọn rác, tải cloudflared.

Sau đó làm nốt 4 việc tay (script in hướng dẫn ở cuối):
1. `sudo nano /etc/img2video/secrets.env` — điền `ANTHROPIC_API_KEY` (dự phòng),
   `ELEVENLABS_API_KEY`, `ALLOWED_ORIGINS`.
2. `sudo -u img2video claude login` — đăng nhập gói Max (nguồn viết kịch bản 0đ).
3. `sudo systemctl restart img2video`.
4. Cloudflared: `tunnel login → create → config.yml → route dns → service install`
   (chi tiết trong output của setup-server.sh).

---

## B. Mỗi lần ra bản mới

**Ở máy local** (nơi con code):
```bash
cd frontend && npm run build      # build ra ../backend/dist
cd ../backend && go test ./...    # phải xanh
cd .. && git add -A && git commit -m "..."   # NHỚ commit kèm backend/dist
git push origin main
```

**Trên server**:
```bash
sudo bash /opt/img2video/deploy/update.sh
```

> ⚠️ Luôn `npm run build` + commit `backend/dist` TRƯỚC khi push. Server không
> build frontend — quên bước này thì giao diện team thấy sẽ là bản cũ.

---

## C. Nguồn Claude viết kịch bản (đã chốt: Max chính + API dự phòng)

Code tự chọn theo thứ tự (narrate.go): (1) `claude` CLI nếu đăng nhập Max →
**0đ**; (2) `ANTHROPIC_API_KEY` nếu Max lỗi/hết hạn → tính tiền; (3) không có gì
→ báo lỗi. Không cần cấu hình thêm, chỉ cần đăng nhập Max + điền API key dự phòng.

**Khi tính năng Thuyết minh AI báo lỗi nguồn Claude** (thường do phiên Max hết hạn):
```bash
sudo -u img2video claude login     # đăng nhập lại; app tự dùng lại Max, KHÔNG cần restart
```

Chi phí API dự phòng (~15k token in + ~800 out/kịch bản; 1 USD ≈ 26.000₫):

| `NARRATE_MODEL` | ₫/kịch bản | Ghi chú |
|---|---|---|
| `claude-opus-4-8` (mặc định) | ~2.500₫ | Chất lượng hài hước tốt nhất — khuyên giữ |
| `claude-sonnet-5` | ~1.000₫ | Rẻ hơn ~60%, đổi 1 dòng env + restart |
| `claude-haiku-4-5` | ~500₫ | Rẻ nhất, humor phẳng hơn |

Đổi model: sửa `NARRATE_MODEL` trong `/etc/img2video/secrets.env` → `sudo systemctl restart img2video`.
Cache: render lại cùng listing/ảnh/settings = 0₫.

---

## D. Checklist go-live (chạy sau khi cài xong)

1. `systemctl status img2video` active + `curl localhost:8080/api/health` OK.
2. Máy khác mở URL cloudflared → giao diện load (chứng minh `dist` + WorkingDirectory đúng).
3. Render **thường** (không thuyết minh) 1 listing → tải video xem được (chứng minh ffmpeg + Chrome không-snap).
4. Vào Cài đặt → bật Thuyết minh AI → Thư viện cảnh đi đường: upload 1 clip; dán
   link Drive 7 clip → chờ status 7/7; soi 1 clip HDR không bệt màu, đúng 1080×1920.
5. Render **thuyết minh + intro** (persona hài hước): có cảnh đi đường + giọng Adam
   đọc hook; render 2 lần → 2 clip intro khác nhau.
6. 2 người render cùng lúc → người sau thấy báo "đang chạy" (giới hạn hiện tại, hàng chờ là bản sau).
7. `sudo reboot` → service + cloudflared tự lên lại.
8. Kiểm tra 2 nguồn Claude: log `journalctl -u img2video -f` khi render thấy đi
   đường CLI (Max); thử `sudo -u img2video claude logout` → render vẫn chạy qua
   API key (fallback sống) → `claude login` lại.
9. `sudo systemctl start img2video-clean.service` chạy tay 1 lần → không xoá nhầm file mới.

---

## E. Lệnh vận hành thường dùng

```bash
sudo systemctl restart img2video          # khởi động lại app
sudo systemctl status img2video           # trạng thái
journalctl -u img2video -f                # xem log realtime
sudo -u img2video claude login            # đăng nhập lại gói Max
sudo nano /etc/img2video/secrets.env      # sửa key / đổi model
```

## F. Sự cố hay gặp

| Hiện tượng | Nguyên nhân | Cách sửa |
|---|---|---|
| Trang trắng / mất font | WorkingDirectory sai | Phải là `/opt/img2video/backend` trong service |
| Overlay/thumbnail đen | Chrome là snap | Dùng google-chrome deb (setup đã lo); kiểm `CHROME_BIN` |
| Thuyết minh AI lỗi "không có nguồn Claude" | Max hết hạn + chưa có API key | `claude login` lại, hoặc điền `ANTHROPIC_API_KEY` |
| Nhập Drive báo lỗi | folder >50 file / Drive throttle | Xem `errors` trong UI; chia nhỏ folder, thử lại sau |
| `update.sh` fail ở git pull | library.json xung đột | Làm theo hướng dẫn `update.sh` in ra |
