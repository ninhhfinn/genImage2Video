#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# setup-server.sh — Cai dat img2video len 1 VPS Linux (Ubuntu/Debian) TU DAU.
# Chay 1 LAN, quyen root:  sudo bash deploy/setup-server.sh
#
# Sau khi xong, moi lan ra ban moi chi can:  sudo bash deploy/update.sh
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

REPO_URL="https://github.com/ninhhfinn/genImage2Video.git"
APP_DIR="/opt/img2video"
APP_USER="img2video"
GO_VERSION="1.25.0"          # phai >= go.mod (go 1.25.0)
VENV_DIR="/opt/img2video-venv"
SECRETS="/etc/img2video/secrets.env"

log() { printf '\n\033[1;36m▶ %s\033[0m\n' "$*"; }
die() { printf '\n\033[1;31m✗ %s\033[0m\n' "$*" >&2; exit 1; }
[ "$(id -u)" = 0 ] || die "Phai chay bang root (sudo)."

ARCH="$(dpkg --print-architecture 2>/dev/null || echo amd64)"   # amd64 | arm64
case "$ARCH" in amd64|arm64) ;; *) die "Chua ho tro arch $ARCH" ;; esac

# ── 1. Goi he thong co ban ──────────────────────────────────────────────────
log "Cai goi he thong (git, ffmpeg, fonts, python venv, curl)…"
apt-get update -y
apt-get install -y git ffmpeg fonts-noto-color-emoji curl ca-certificates python3-venv jq

# HDR tonemap can libzimg (filter zscale) — kiem tra ffmpeg co ho tro khong.
if ! ffmpeg -hide_banner -filters 2>/dev/null | grep -q ' zscale '; then
  echo "⚠ ffmpeg thieu filter zscale (libzimg): clip HDR iPhone se bi bet mau."
  echo "  Cai ffmpeg day du (vd tu deb-multimedia) neu can HDR chuan. Tam thoi van chay duoc voi clip SDR."
fi

# ── 2. Chrome (KHONG dung snap — snap khong doc duoc /tmp -> overlay den) ─────
log "Cai trinh duyet cho render overlay/thumbnail…"
CHROME_BIN_VAL="google-chrome"
if grep -qi ubuntu /etc/os-release; then
  # Ubuntu: apt chromium la snap -> dung google-chrome-stable tu repo Google.
  install -d -m 0755 /etc/apt/keyrings
  curl -fsSL https://dl.google.com/linux/linux_signing_key.pub | gpg --dearmor -o /etc/apt/keyrings/google-chrome.gpg
  echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/google-chrome.gpg] https://dl.google.com/linux/chrome/deb/ stable main" \
    > /etc/apt/sources.list.d/google-chrome.list
  apt-get update -y
  if ! apt-get install -y google-chrome-stable; then
    echo "⚠ Khong cai duoc google-chrome (co the arm64). Thu chromium deb…"
    apt-get install -y chromium && CHROME_BIN_VAL="chromium"
  fi
else
  # Debian: apt chromium la deb that -> dung duoc.
  apt-get install -y chromium
  CHROME_BIN_VAL="chromium"
fi

# ── 3. Go (tarball chinh thuc — apt qua cu so voi go 1.25) ───────────────────
if ! /usr/local/go/bin/go version 2>/dev/null | grep -q "go${GO_VERSION}"; then
  log "Cai Go ${GO_VERSION} (tarball)…"
  TARBALL="go${GO_VERSION}.linux-${ARCH}.tar.gz"
  curl -fsSL "https://go.dev/dl/${TARBALL}" -o "/tmp/${TARBALL}"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "/tmp/${TARBALL}"
  ln -sf /usr/local/go/bin/go /usr/local/bin/go
  ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
fi
go version

# ── 4. gdown (nhap clip tu Google Drive) — venv vi PEP 668 ───────────────────
log "Cai gdown (nhap clip intro tu Drive)…"
python3 -m venv "$VENV_DIR"
"$VENV_DIR/bin/pip" install --upgrade pip >/dev/null
"$VENV_DIR/bin/pip" install gdown >/dev/null
ln -sf "$VENV_DIR/bin/gdown" /usr/local/bin/gdown
gdown --version || true

# ── 5. User + clone repo ────────────────────────────────────────────────────
log "Tao user $APP_USER + clone repo…"
id "$APP_USER" >/dev/null 2>&1 || useradd -r -m -s /usr/sbin/nologin "$APP_USER"
if [ ! -d "$APP_DIR/.git" ]; then
  git clone "$REPO_URL" "$APP_DIR"
fi
chown -R "$APP_USER:$APP_USER" "$APP_DIR"

# library.json bi server ghi (like kich ban) -> git pull khoi conflict.
sudo -u "$APP_USER" git -C "$APP_DIR" update-index --skip-worktree backend/assets/scripts/library.json 2>/dev/null || true

# ── 6. Secrets template ─────────────────────────────────────────────────────
log "Tao file bi mat $SECRETS…"
install -d -m 0750 -o root -g "$APP_USER" /etc/img2video
if [ ! -f "$SECRETS" ]; then
  cat > "$SECRETS" <<EOF
# API key du phong (khi phien Claude Code goi Max het han). De trong neu chi dung Max.
ANTHROPIC_API_KEY=
# Giong doc ElevenLabs (Adam free tier). Bat buoc neu muon giong Adam; thieu -> tu roi Google free.
ELEVENLABS_API_KEY=
ELEVENLABS_VOICE_ID=
# Model viet kich ban. Bo trong = claude-opus-4-8. Doi claude-sonnet-5 de giam ~60% chi phi.
NARRATE_MODEL=claude-opus-4-8
PORT=8080
# CORS: domain web (qua cloudflared). Vd https://video.dayladau.com
ALLOWED_ORIGINS=
CHROME_BIN=${CHROME_BIN_VAL}
EOF
  chmod 640 "$SECRETS"; chown root:"$APP_USER" "$SECRETS"
  echo "→ NHO dien ANTHROPIC_API_KEY + ELEVENLABS_API_KEY + ALLOWED_ORIGINS vao $SECRETS"
fi

# ── 7. Claude Code goi Max (nguon chinh, 0d) ────────────────────────────────
log "Cai Claude Code (goi Max — nguon viet kich ban 0d)…"
sudo -u "$APP_USER" bash -lc 'curl -fsSL https://claude.ai/install.sh | bash' || \
  echo "⚠ Cai claude that bai — bo qua, app se dung ANTHROPIC_API_KEY thay the."
# Symlink de service (LookPath \"claude\") thay binary.
CLAUDE_BIN="$(sudo -u "$APP_USER" bash -lc 'command -v claude || echo "$HOME/.local/bin/claude"')"
[ -x "$CLAUDE_BIN" ] && ln -sf "$CLAUDE_BIN" /usr/local/bin/claude || true
echo "→ Dang nhap Max sau khi setup xong:  sudo -u $APP_USER claude login   (hoac: claude setup-token)"

# ── 8. Build lan dau ────────────────────────────────────────────────────────
log "Build backend lan dau…"
sudo -u "$APP_USER" bash -lc "cd $APP_DIR/backend && /usr/local/go/bin/go build -o img2video ."

# ── 9. systemd unit + timer don rac ─────────────────────────────────────────
log "Cai systemd service + timer…"
install -m 0644 "$APP_DIR/deploy/img2video.service"       /etc/systemd/system/img2video.service
install -m 0644 "$APP_DIR/deploy/img2video-clean.service" /etc/systemd/system/img2video-clean.service
install -m 0644 "$APP_DIR/deploy/img2video-clean.timer"   /etc/systemd/system/img2video-clean.timer
systemctl daemon-reload
systemctl enable --now img2video.service
systemctl enable --now img2video-clean.timer

# ── 10. cloudflared (URL cong khai on dinh, khong mo port) ───────────────────
log "Cai cloudflared (tunnel cong khai)…"
if ! command -v cloudflared >/dev/null; then
  curl -fsSL "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${ARCH}.deb" -o /tmp/cloudflared.deb
  apt-get install -y /tmp/cloudflared.deb || dpkg -i /tmp/cloudflared.deb || true
fi
cat <<'NOTE'

┌─ CLOUDFLARED — lam THU CONG sau khi setup (can domain tren Cloudflare) ─────┐
│ 1) cloudflared tunnel login                                                 │
│ 2) cloudflared tunnel create img2video                                      │
│ 3) Tao /etc/cloudflared/config.yml:                                         │
│      tunnel: <tunnel-id>                                                     │
│      credentials-file: /root/.cloudflared/<tunnel-id>.json                  │
│      ingress:                                                               │
│        - hostname: video.<domain-cua-ban>                                   │
│          service: http://localhost:8080                                     │
│        - service: http_status:404                                           │
│ 4) cloudflared tunnel route dns img2video video.<domain>                    │
│ 5) cloudflared service install   (chay nen, tu bat khi reboot)              │
│ (Tuy chon) Cloudflare Access: chi cho email @dayladau.com vao — 0 dong code │
│                                                                             │
│ KHONG co domain? Tam thoi:  cloudflared tunnel --url http://localhost:8080  │
│   (URL ngau nhien, doi moi lan chay — chi hop test).                        │
└─────────────────────────────────────────────────────────────────────────────┘

✓ XONG PHAN TU DONG. Viec con lai (lam 1 lan):
  1. Dien key vao:            sudo nano /etc/img2video/secrets.env
  2. Dang nhap Claude Max:    sudo -u img2video claude login
  3. Restart:                 sudo systemctl restart img2video
  4. Cloudflared theo huong dan o tren.
  5. Kiem tra:                curl localhost:8080/api/health
NOTE
