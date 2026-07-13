# img2video — Hướng dẫn chạy

## Cấu trúc thư mục
```
img2video/
├── backend/     ← Go API server
└── frontend/    ← React UI
```

---

## Lần đầu cài đặt

### Yêu cầu
- [Go 1.20+](https://go.dev/dl/) — go.dev/dl
- [Node.js LTS](https://nodejs.org) — nodejs.org  
- [FFmpeg](https://ffmpeg.org/download.html) — ffmpeg.org

Trên Ubuntu:
```
sudo apt update
sudo apt install -y ffmpeg
ffmpeg -version
```

### Cài frontend (chỉ cần làm 1 lần)
```
Mở terminal → vào thư mục frontend → chạy:
npm install
```

---

## Chạy mỗi ngày (2 terminal)

### Terminal 1 — Backend
```
cd backend
go run . --web --port 8080
```

### Terminal 2 — Frontend
```
cd frontend
npm run dev
```

Mở trình duyệt: **http://localhost:5173**

---

## Build production (1 file chạy luôn)
```
cd frontend
npm run build

cd ..\backend
go build -o img2video.exe .
.\img2video.exe --web
```
Mở: **http://localhost:8080**

---

## 🎙️ Mode "Thuyết minh AI" (video có lời kể + giọng đọc)

Chế độ này để AI **tự nhìn ảnh → viết lời kể tiếng Việt → lồng giọng đọc + phụ đề**,
mỗi ảnh hiện đúng bằng thời gian giọng nói về nó (kèm nhạc nền tự nhỏ khi đang nói).

### Cần khoá API (đặt biến môi trường TRƯỚC khi chạy backend)

| Việc | Biến môi trường | Ghi chú |
|---|---|---|
| AI viết kịch bản | *(không cần gì)* nếu máy đã đăng nhập **Claude Code** (lệnh `claude`) → dùng gói của bạn, **0đ** | Ưu tiên tự động. |
| — hoặc — | `ANTHROPIC_API_KEY` | Chỉ cần khi máy **không** có Claude Code. Tính phí theo token. |
| **Giọng Google (free)** | *(không cần key)* | 🆓 Đọc tiếng Việt miễn phí, không cần đăng ký. Giọng máy (robot) nhưng rõ — hợp thử nhanh. Chọn "🆓 Google (free)" trong UI. |
| Giọng ElevenLabs | `ELEVENLABS_API_KEY`, (tuỳ chọn) `ELEVENLABS_VOICE_ID` | ⚠️ Gói **free KHÔNG dùng được giọng thư viện lẫn tạo/clone giọng qua API** — cần nâng cấp (~$5/tháng). Giọng hay nhất, biểu cảm nhất. Mặc định dùng giọng Việt theo persona. |
| Giọng FPT.AI | `FPT_TTS_API_KEY`, (tuỳ chọn) `FPT_TTS_VOICE` | Rẻ, thuần Việt, đọc số chuẩn. |

**Gợi ý chọn giọng:** muốn **miễn phí ngay, không đăng ký** → chọn **Google**. Muốn giọng thật/xịn → FPT.AI (rẻ) hoặc ElevenLabs (hay nhất, cần gói trả phí).

Ví dụ chạy backend có khoá (Linux/macOS):
```
cd backend
export ELEVENLABS_API_KEY="sk-..."      # hoặc export FPT_TTS_API_KEY="..."
# (không bắt buộc ANTHROPIC_API_KEY nếu đã có Claude Code)
go run . --web --port 8080
```

### Dùng
1. Chọn 1 phòng như bình thường (tab Dayladau / URL / Upload).
2. Ở phần **Hiệu ứng**, chọn **🎙️ Thuyết minh AI**.
3. Tuỳ chọn: giọng văn (hài hước / lịch sự), nhà cung cấp giọng, voice, số cảnh, nhạc nền.
4. Bấm tạo video như thường. Backend sẽ: viết kịch bản → lồng giọng → ghép video.

Yêu cầu: **FFmpeg ≥ 4.4** (ducking nhạc nền dùng `sidechaincompress`).

### Chi phí mỗi video (ước tính)
- Kịch bản: **0đ** nếu dùng Claude Code; hoặc ~800đ–4.000đ nếu dùng API key (tuỳ model).
- Giọng đọc: ElevenLabs ~1 video trong hạn mức free; FPT.AI rẻ hơn.
- Render lại cùng một phòng: **không gọi lại API** (đã cache kịch bản + giọng).

---

## Hỗ trợ
- Gặp lỗi → chụp màn hình terminal gửi để được hỗ trợ
