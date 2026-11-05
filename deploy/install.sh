#!/usr/bin/env bash
set -euo pipefail

[[ ${EUID} -eq 0 ]] || { echo 'باید با root اجرا شود.' >&2; exit 1; }
command -v go >/dev/null || { echo 'Go نصب نیست.' >&2; exit 1; }

APP=/opt/rabin-assistant
DATA=/var/lib/rabin-assistant
CONF=/etc/rabin-assistant.env

id rabin-assistant >/dev/null 2>&1 || useradd --system --home-dir "$APP" --shell /usr/sbin/nologin rabin-assistant
install -d -o root -g root -m 0755 "$APP"
install -d -o rabin-assistant -g rabin-assistant -m 0700 "$DATA"

if [[ ! -f "$CONF" ]]; then
  PASS=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
  umask 077
  printf 'RABIN_ADMIN_USER=admin\nRABIN_ADMIN_PASS=%s\nRABIN_ADDR=127.0.0.1:8098\nRABIN_DATA_DIR=%s\n' "$PASS" "$DATA" > "$CONF"
  chmod 600 "$CONF"
  printf 'Rabin Assistant credentials\nUsername: admin\nPassword: %s\nURL (via SSH tunnel): http://127.0.0.1:8098\n' "$PASS" > /root/rabin-assistant-credentials.txt
  chmod 600 /root/rabin-assistant-credentials.txt
  echo 'اطلاعات ورود در /root/rabin-assistant-credentials.txt ذخیره شد.'
else
  echo "تنظیمات قبلی حفظ شد: $CONF"
fi

cd "$(dirname "$0")/.."
go build -trimpath -ldflags='-s -w' -o "$APP/rabin-assistant" ./cmd/server
chmod 0755 "$APP/rabin-assistant"
install -m 0644 deploy/rabin-assistant.service /etc/systemd/system/rabin-assistant.service
systemctl daemon-reload
systemctl enable --now rabin-assistant.service
systemctl --no-pager --full status rabin-assistant.service || true

echo 'اتصال امن از رایانه خود:'
echo 'ssh -L 8098:127.0.0.1:8098 root@IP-SERVER'
echo 'سپس http://127.0.0.1:8098 را باز کنید.'
