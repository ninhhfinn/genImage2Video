#!/usr/bin/env bash
# ============================================================
#  Khởi động web Gen Video + tạo link public (Cloudflare)
#  Cách dùng:  ./start-web.sh
#  Tắt:        nhấn Ctrl + C
# ============================================================
cd "$(dirname "$0")/backend" || exit 1

# 1) Khởi động backend nếu chưa chạy ------------------------
if curl -s -o /dev/null http://localhost:8080/api/health 2>/dev/null; then
  echo "✅ Backend đã chạy sẵn ở http://localhost:8080"
else
  echo "▶  Đang khởi động backend..."
  ./image2video --web > /tmp/genvideo-backend.log 2>&1 &
  for i in $(seq 1 15); do
    curl -s -o /dev/null http://localhost:8080/api/health 2>/dev/null && break
    sleep 1
  done
  echo "✅ Backend chạy ở http://localhost:8080"
fi

# 2) Bật link public qua Cloudflare Tunnel (link CỐ ĐỊNH) ---
echo "▶  Đang bật link public..."
~/.local/bin/cloudflared tunnel --no-autoupdate run genvideo \
    > /tmp/genvideo-tunnel.log 2>&1 &
TUNNEL_PID=$!

# 3) Link CỐ ĐỊNH — không đổi mỗi lần -----------------------
URL="https://video.quanlyhomestay.com"
# chờ tunnel kết nối xong rồi mới in link
for i in $(seq 1 25); do
  grep -q "Registered tunnel connection" /tmp/genvideo-tunnel.log 2>/dev/null && break
  sleep 1
done

echo
echo "=================================================================="
if [ -n "$URL" ]; then
  echo "  🌐  WEB CỦA BẠN ĐÃ LIVE:"
  echo
  echo "      $URL"
else
  echo "  ⚠️  Chưa lấy được link. Xem log: /tmp/genvideo-tunnel.log"
fi
echo "=================================================================="
echo "  •  Gửi link này cho người khác để họ dùng web."
echo "  •  GIỮ CỬA SỔ NÀY MỞ — đóng lại là web tắt."
echo "  •  Nhấn Ctrl + C để tắt link public."
echo "  •  ✅ Link CỐ ĐỊNH — luôn giống nhau mỗi lần bật."
echo "=================================================================="
echo

wait $TUNNEL_PID
