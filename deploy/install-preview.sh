#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo 'این نصب‌کننده باید با root اجرا شود.' >&2
  exit 1
fi
command -v systemctl >/dev/null 2>&1 || {
  echo 'systemd روی این سرور در دسترس نیست.' >&2
  exit 1
}

for command_name in go; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo 'Go نصب نیست؛ نصب Go از مخازن Ubuntu...'
    apt-get update
    apt-get install -y golang-go
  fi
done

GO_VERSION="$(go env GOVERSION | sed 's/^go//')"
MIN_VERSION="1.23"
if [[ "$(printf '%s\n%s\n' "$MIN_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$MIN_VERSION" ]]; then
  echo "Go 1.23 یا جدیدتر لازم است؛ نسخه فعلی: $GO_VERSION" >&2
  exit 1
fi

APP=/opt/rabin-assistant-preview
DATA=/var/lib/rabin-assistant-preview
CONF=/etc/rabin-assistant-preview.env
KEY=/etc/rabin-assistant-preview.key
UNIT=rabin-assistant-preview.service
RUN_USER=rabin-assistant-preview
PORT=8099

id "$RUN_USER" >/dev/null 2>&1 || useradd --system --home-dir "$APP" --shell /usr/sbin/nologin "$RUN_USER"
install -d -o root -g root -m 0755 "$APP"
install -d -o "$RUN_USER" -g "$RUN_USER" -m 0700 "$DATA"

# A dedicated encryption key keeps preview credentials isolated from production.
if [[ ! -e "$KEY" ]]; then
  umask 077
  head -c 32 /dev/urandom > "$KEY"
elif [[ ! -f "$KEY" ]] || [[ "$(wc -c < "$KEY")" -ne 32 ]]; then
  echo "کلید رمزنگاری $KEY باید فایل معمولی دقیقاً ۳۲ بایتی باشد؛ نصب متوقف شد." >&2
  exit 1
fi
chown root:"$RUN_USER" "$KEY"
chmod 0640 "$KEY"

if [[ ! -f "$CONF" ]]; then
  PASS="$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')"
  umask 077
  printf 'RABIN_ADMIN_USER=preview\nRABIN_ADMIN_PASS=%s\n' "$PASS" > "$CONF"
  printf 'Rabin Assistant PREVIEW credentials\nUsername: preview\nPassword: %s\nURL: http://SERVER_IP:%s\n' "$PASS" "$PORT" > /root/rabin-assistant-preview-credentials.txt
  chmod 600 /root/rabin-assistant-preview-credentials.txt
  echo 'اطلاعات ورود نسخه آزمایشی در /root/rabin-assistant-preview-credentials.txt ذخیره شد.'
else
  echo "تنظیمات ورود قبلی نسخه آزمایشی حفظ شد: $CONF"
fi

set_env() {
  local key="$1" value="$2"
  if grep -q "^\${key}=" "$CONF"; then
    sed -i "s|^\${key}=.*|\${key}=\${value}|" "$CONF"
  else
    printf '%s=%s\n' "$key" "$value" >> "$CONF"
  fi
}

# Always enforce isolated storage, dedicated port/key, and disabled live orders.
set_env RABIN_ADDR "0.0.0.0:$PORT"
set_env RABIN_DATA_DIR "$DATA"
set_env RABIN_CREDENTIALS_KEY_FILE "$KEY"
set_env RABIN_ENABLE_LIVE_ORDER_SUBMISSION "false"
chmod 600 "$CONF"

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

gofmt -w cmd/server/*.go

echo 'اجرای تست‌های خودکار نسخه آزمایشی...'
go test ./...

echo 'کامپایل نسخه آزمایشی...'
go build -trimpath -ldflags='-s -w' -o "$APP/rabin-assistant" ./cmd/server
chmod 0755 "$APP/rabin-assistant"

cat > "/etc/systemd/system/$UNIT" <<'UNIT_FILE'
[Unit]
Description=Rabin Assistant PREVIEW on port 8099
After=network.target

[Service]
Type=simple
User=rabin-assistant-preview
Group=rabin-assistant-preview
WorkingDirectory=/opt/rabin-assistant-preview
EnvironmentFile=/etc/rabin-assistant-preview.env
ExecStart=/opt/rabin-assistant-preview/rabin-assistant
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
ReadWritePaths=/var/lib/rabin-assistant-preview
LockPersonality=true
MemoryDenyWriteExecute=true

[Install]
WantedBy=multi-user.target
UNIT_FILE

systemctl daemon-reload
systemctl enable "$UNIT"
systemctl restart "$UNIT"

echo
echo 'نصب نسخه آزمایشی کامل شد؛ سرویس اصلی rabin-assistant.service دست‌نخورده باقی مانده است.'
systemctl --no-pager --full status "$UNIT" || true
echo
echo 'اطلاعات ورود: cat /root/rabin-assistant-preview-credentials.txt'
echo 'پنل آزمایشی: http://SERVER_IP:8099'
echo 'ارسال واقعی API در نسخه آزمایشی اجباری و خاموش است.'
