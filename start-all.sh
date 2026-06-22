#!/usr/bin/env bash
# ============================================================
#  start-all.sh — Bật web Gen Video + link public chỉ bằng 1 lệnh
#
#  Quy trình:
#    1) Build frontend  → backend/dist   (giao diện mới nhất, gồm nền sáng/tối)
#    2) Chạy backend     :8080           (phục vụ dist + API)
#    3) start-web.sh                     (tạo link public qua Cloudflare)
#
#  Cách dùng:  ./start-all.sh
#  Tắt:        nhấn Ctrl + C  (tự tắt backend nếu script này đã bật nó)
#
#  Vì link public (start-web.sh) trỏ vào :8080 = backend phục vụ backend/dist,
#  nên BẮT BUỘC build frontend trước thì web public mới hiện giao diện mới.
# ============================================================
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

BACKEND_PID=""
STARTED_BACKEND=0
cleanup() {
  if [ "$STARTED_BACKEND" = "1" ] && [ -n "$BACKEND_PID" ]; then
    echo
    echo "🧹  Đang tắt backend (do script này bật)..."
    pkill -P "$BACKEND_PID" 2>/dev/null || true   # tiến trình server con của 'go run'
    kill "$BACKEND_PID"    2>/dev/null || true     # chính 'go run'
  fi
}
trap cleanup EXIT INT TERM

# ── Kiểm tra công cụ cần thiết ─────────────────────────────
need() { command -v "$1" >/dev/null 2>&1 || { echo "❌  Thiếu '$1' — hãy cài trước."; exit 1; }; }
need go
need npm
[ -x "$HOME/.local/bin/cloudflared" ] || command -v cloudflared >/dev/null 2>&1 \
  || { echo "❌  Thiếu cloudflared (~/.local/bin/cloudflared) — cần để tạo link public."; exit 1; }

# ── 1) Build frontend → backend/dist ───────────────────────
echo "🔨  [1/3] Build giao diện (frontend → backend/dist)..."
( cd frontend && npm run build )
echo "✅  Build xong → backend/dist"

# ── 2) Backend :8080 (phục vụ dist + API) ──────────────────
if curl -sf http://localhost:8080/api/health >/dev/null 2>&1; then
  echo "✅  [2/3] Backend đã chạy sẵn ở http://localhost:8080 — dùng lại."
else
  echo "▶   [2/3] Khởi động backend: go run . --web --port 8080"
  # exec trong subshell → \$! chính là PID của 'go run' (server là tiến trình con của nó)
  ( cd backend && exec go run . --web --port 8080 ) > /tmp/genvideo-backend.log 2>&1 &
  BACKEND_PID=$!
  STARTED_BACKEND=1
  ok=0
  for _ in $(seq 1 90); do
    if curl -sf http://localhost:8080/api/health >/dev/null 2>&1; then ok=1; break; fi
    if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
      echo "❌  Backend dừng khi khởi động. Log cuối:"; tail -n 40 /tmp/genvideo-backend.log; exit 1
    fi
    sleep 1
  done
  [ "$ok" = "1" ] || { echo "❌  Backend không phản hồi sau 90s. Log:"; tail -n 40 /tmp/genvideo-backend.log; exit 1; }
  echo "✅  Backend chạy ở http://localhost:8080  (log: /tmp/genvideo-backend.log)"
fi

# ── 3) Link public qua Cloudflare ──────────────────────────
echo "🌐  [3/3] Tạo link public bằng Cloudflare (start-web.sh)..."
echo "     Backend đã sẵn sàng → start-web.sh chỉ tạo tunnel."
echo "     ⚠  Giữ cửa sổ này mở. Nhấn Ctrl+C để tắt link (và backend nếu script bật)."
./start-web.sh
