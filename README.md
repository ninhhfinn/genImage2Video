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

## Hỗ trợ
- Gặp lỗi → chụp màn hình terminal gửi để được hỗ trợ
