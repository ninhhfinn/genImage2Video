#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# update.sh — Cap nhat img2video len ban moi tren SERVER. Chay:  sudo bash deploy/update.sh
# (Da chay setup-server.sh 1 lan truoc do.)
#
# Quy trinh: keo code moi tu GitHub -> build lai -> restart service.
# Server KHONG can Node: frontend/dist da duoc build san & commit o may local.
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail
APP_DIR="/opt/img2video"
APP_USER="img2video"
log() { printf '\n\033[1;36m▶ %s\033[0m\n' "$*"; }

log "Keo code moi (git pull)…"
if ! sudo -u "$APP_USER" git -C "$APP_DIR" pull --ff-only; then
  cat <<'MSG'
✗ git pull that bai. Nguyen nhan hay gap: backend/assets/scripts/library.json
  bi sua o server (do like kich ban) dung upstream cung sua.
  Cach xu ly:
    sudo -u img2video git -C /opt/img2video update-index --no-skip-worktree backend/assets/scripts/library.json
    sudo -u img2video git -C /opt/img2video stash
    sudo -u img2video git -C /opt/img2video pull --ff-only
    sudo -u img2video git -C /opt/img2video stash pop   # neu muon giu like server
    sudo -u img2video git -C /opt/img2video update-index --skip-worktree backend/assets/scripts/library.json
MSG
  exit 1
fi

log "Build backend…"
sudo -u "$APP_USER" bash -lc "cd $APP_DIR/backend && /usr/local/go/bin/go build -o img2video ."

log "Restart service…"
systemctl restart img2video
sleep 1
systemctl --no-pager --lines=5 status img2video || true
echo
echo "✓ Xong. Kiem tra: curl localhost:8080/api/health"
