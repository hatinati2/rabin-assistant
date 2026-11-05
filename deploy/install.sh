#!/usr/bin/env bash
set -euo pipefail

[[ ${EUID} -eq 0 ]] || { echo 'این نصب‌کننده باید با root اجرا شود.' >&2; exit 1; }
command -v systemctl >/dev/null || { echo 'systemd روی این سرور در دسترس نیست.' >&2; exit 1; }

if ! command -v go >/dev/null 2>&1; then
  echo 'Go نصب نیست؛ نصب Go از مخازن Ubuntu...'
  apt-get update
  apt-get install -y golang-go
fi

GO_VERSION="$(go env GOVERSION | sed 's/^go//')"
MIN_VERSION="1.23"
if [[ "$(printf '%s\n%s\n' "$MIN_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$MIN_VERSION" ]]; then
  echo "Go 1.23 یا جدیدتر لازم است؛ نسخه فعلی: $GO_VERSION" >&2
  exit 1
fi

APP=/opt/rabin-assistant
DATA=/var/lib/rabin-assistant
CONF=/etc/rabin-assistant.env

id rabin-assistant >/dev/null 2>&1 || useradd --system --home-dir "$APP" --shell /usr/sbin/nologin rabin-assistant
install -d -o root -g root -m 0755 "$APP"
install -d -o rabin-assistant -g rabin-assistant -m 0700 "$DATA"

if [[ ! -f "$CONF" ]]; then
  PASS="$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')"
  umask 077
  printf 'RABIN_ADMIN_USER=admin\nRABIN_ADMIN_PASS=%s\nRABIN_ADDR=0.0.0.0:8098\nRABIN_DATA_DIR=%s\n' "$PASS" "$DATA" > "$CONF"
  chmod 600 "$CONF"
  printf 'Rabin Assistant credentials\nUsername: admin\nPassword: %s\nURL (via SSH tunnel): http://127.0.0.1:8098\n' "$PASS" > /root/rabin-assistant-credentials.txt
  chmod 600 /root/rabin-assistant-credentials.txt
  echo 'اطلاعات ورود در /root/rabin-assistant-credentials.txt ذخیره شد.'
else
  echo "تنظیمات قبلی حفظ شد: $CONF"
fi

# Update the previous default so this test build can be reached by server IP.
if grep -q '^RABIN_ADDR=127\\.0\\.0\\.1:8098"$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"
go build -trimpath -ldflags='-s -w' -o "$APP/rabin-assistant" ./cmd/server
chmod 0755 "$APP/rabin-assistant"
install -m 0644 "$SCRIPT_DIR/rabin-assistant.service" /etc/systemd/system/rabin-assistant.service
systemctl daemon-reload
systemctl enable --now rabin-assistant.service

echo
echo 'نصب کامل شد.'
echo 'وضعیت سرویس:'
systemctl --no-pager --full status rabin-assistant.service || true
echo
echo 'اطلاعات ورود:'
echo '  cat /root/rabin-assistant-credentials.txt'
echo
echo 'اتصال امن از رایانه:'
echo '  ssh -L 8098:127.0.0.1:8098 root@IP-SERVER'
echo 'سپس http://127.0.0.1:8098 را باز کنید.'
 "$CONF"; then
  sed -i 's/^RABIN_ADDR=127\\.0\\.0\\.1:8098$/RABIN_ADDR=0.0.0.0:8098/' "$CONF"
elif ! grep -q '^RABIN_ADDR=' "$CONF"; then
  printf '\\nRABIN_ADDR=0.0.0.0:8098\\n' >> "$CONF"
fi
chmod 600 "$CONF"

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"
go build -trimpath -ldflags='-s -w' -o "$APP/rabin-assistant" ./cmd/server
chmod 0755 "$APP/rabin-assistant"
install -m 0644 "$SCRIPT_DIR/rabin-assistant.service" /etc/systemd/system/rabin-assistant.service
systemctl daemon-reload
systemctl enable --now rabin-assistant.service

echo
echo 'نصب کامل شد.'
echo 'وضعیت سرویس:'
systemctl --no-pager --full status rabin-assistant.service || true
echo
echo 'اطلاعات ورود:'
echo '  cat /root/rabin-assistant-credentials.txt'
echo
echo 'اتصال امن از رایانه:'
echo '  ssh -L 8098:127.0.0.1:8098 root@IP-SERVER'
echo 'سپس http://127.0.0.1:8098 را باز کنید.'
