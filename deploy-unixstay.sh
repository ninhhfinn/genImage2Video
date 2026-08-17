#!/usr/bin/env bash
# ============================================================================
#  Cập nhật app Dọn dẹp lên bản mới nhất — chạy trên máy đang phục vụ web.
#
#  Dùng: ./deploy-unixstay.sh
#
#  Script làm đúng những việc dễ quên khi làm tay: kéo code mới, build CẢ hai
#  phía, tắt tiến trình cũ trước khi chạy tiến trình mới, và KIỂM TRA lại xem
#  trình duyệt thật sự nhận được app đúng — vì HTTP 200 không chứng minh app
#  chạy được (đã gặp: trang trắng do file JS bị trả về nhầm nội dung).
# ============================================================================
set -euo pipefail

cd "$(dirname "$0")"
PORT="${PORT:-8080}"

say() { printf "\n\033[1;36m▶ %s\033[0m\n" "$1"; }
ok()  { printf "  \033[32m✓\033[0m %s\n" "$1"; }
die() { printf "  \033[31m✗ %s\033[0m\n" "$1"; exit 1; }

# ─── 1. Lấy code mới ────────────────────────────────────────────────────────
say "Kéo code mới từ GitHub"
git pull --ff-only origin master || die "git pull hỏng — có thay đổi chưa commit trên máy này?"
ok "$(git log --oneline -1)"

# ─── 2. Build giao diện ─────────────────────────────────────────────────────
say "Build app Dọn dẹp"
cd frontend-admin
npm ci --no-audit --no-fund 2>/dev/null || npm install --no-audit --no-fund
# npm hay bỏ sót gói native theo nền tảng; không có nó thì build chết giữa chừng.
if ! npm run build >/tmp/unixstay-ui.log 2>&1; then
  echo "  (thử cài gói native còn thiếu rồi build lại)"
  npm install --no-save "@rolldown/binding-$(node -p "process.platform+'-'+process.arch")@$(node -p "require('./node_modules/rolldown/package.json').version")" >/dev/null 2>&1 || true
  npm run build >/tmp/unixstay-ui.log 2>&1 || { tail -20 /tmp/unixstay-ui.log; die "build giao diện hỏng"; }
fi
cd ..
[ -f backend/dist-admin/index.html ] || die "không thấy backend/dist-admin/index.html"
ok "giao diện xong"

# ─── 3. Build backend ───────────────────────────────────────────────────────
say "Build backend"
cd backend
go build -o image2video . || die "build Go hỏng"
cd ..
ok "binary xong"

# ─── 4. Khởi động lại ───────────────────────────────────────────────────────
say "Khởi động lại máy chủ"
# Tắt tiến trình cũ trước, nếu không cổng bị chiếm và bản mới không lên được —
# lỗi này im lặng: web vẫn chạy, nhưng vẫn là code cũ.
pkill -f 'image2video --web' 2>/dev/null || true
sleep 1

: "${HK_ADMIN_PHONE:=}"
: "${HK_ADMIN_PASSWORD:=}"
export HK_DATA_DIR="${HK_DATA_DIR:-$PWD/hk_data}"

cd backend
nohup ./image2video --web --port "$PORT" > /tmp/unixstay-server.log 2>&1 &
cd ..

for _ in $(seq 1 20); do
  curl -sf -o /dev/null "http://localhost:$PORT/api/health" && break
  sleep 1
done
curl -sf -o /dev/null "http://localhost:$PORT/api/health" || { tail -20 /tmp/unixstay-server.log; die "máy chủ không lên"; }
ok "máy chủ chạy ở cổng $PORT — dữ liệu tại $HK_DATA_DIR"

# ─── 5. Kiểm tra thật ───────────────────────────────────────────────────────
say "Kiểm tra app tới được trình duyệt"
BASE="http://localhost:$PORT"

TITLE=$(curl -s -H "Host: unixstay.quanlyhomestay.com" "$BASE/" | grep -o '<title>[^<]*</title>' || true)
case "$TITLE" in
  *"Dọn dẹp"*) ok "tên miền unixstay → app Dọn dẹp" ;;
  *) die "tên miền unixstay đang trả: ${TITLE:-(không có title)}" ;;
esac

# Nạp từng file mà trang cần: 200 trên HTML không chứng minh app chạy được.
for A in $(curl -s -H "Host: unixstay.quanlyhomestay.com" "$BASE/" | grep -oE '/assets-admin/[^"]+'); do
  CT=$(curl -s -o /dev/null -w '%{content_type}' "$BASE$A")
  case "$A:$CT" in
    *.js:*javascript*|*.css:*text/css*) ok "$(basename "$A")" ;;
    *) die "$A trả sai kiểu: $CT" ;;
  esac
done

curl -sf -o /dev/null "$BASE/api/hk/meta" -w '' 2>/dev/null
CT=$(curl -s -o /dev/null -w '%{content_type}' "$BASE/api/hk/meta")
case "$CT" in
  *json*) ok "API module Dọn dẹp trả JSON" ;;
  *) die "/api/hk/meta trả $CT — binary chưa có module Dọn dẹp?" ;;
esac

TITLE=$(curl -s "$BASE/" | grep -o '<title>[^<]*</title>' || true)
case "$TITLE" in
  *img2video*) ok "công cụ video ở gốc vẫn nguyên" ;;
  *) printf "  \033[33m! công cụ video trả: %s (chưa build frontend/ ?)\033[0m\n" "${TITLE:-trống}" ;;
esac

printf "\n\033[1;32m✅ Xong.\033[0m  https://unixstay.quanlyhomestay.com\n"
printf "   Log máy chủ: /tmp/unixstay-server.log\n\n"
