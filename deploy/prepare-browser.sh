#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${EUID}" -ne 0 ]]; then
  echo "این آماده‌ساز باید با root اجرا شود." >&2
  exit 1
fi
if ! id rabin-assistant >/dev/null 2>&1; then
  echo "کاربر سیستمی rabin-assistant پیدا نشد؛ ابتدا نصب فعلی Rabin Assistant را بررسی کن." >&2
  exit 1
fi

APP_DIR="/opt/rabin-assistant"
BROWSER_DIR="$APP_DIR/browser"
VENV="$BROWSER_DIR/venv"
BROWSERS="$BROWSER_DIR/browsers"
DATA_DIR="/var/lib/rabin-assistant"
PY_URL="https://raw.githubusercontent.com/hatinati2/rabin-assistant/main/deploy/browser-preflight.py"

echo "نصب پیش‌نیازهای مرورگر آزمایشی..."
apt-get update
apt-get install -y python3 python3-venv python3-pip curl

install -d -o rabin-assistant -g rabin-assistant -m 0750 "$BROWSER_DIR" "$BROWSERS"
install -d -o rabin-assistant -g rabin-assistant -m 0700 "$DATA_DIR"

if [[ ! -x "$VENV/bin/python" ]]; then
  runuser -u rabin-assistant -- python3 -m venv "$VENV"
fi

# Updating pip is unnecessary here and can fail on restricted/unstable networks.
# Retry package downloads with a longer timeout and without a per-user cache.
runuser -u rabin-assistant -- env PIP_DISABLE_PIP_VERSION_CHECK=1 PIP_NO_CACHE_DIR=1 \
  "$VENV/bin/python" -m pip install --retries 8 --timeout 120 playwright

echo "نصب کتابخانه‌های سیستمی Chromium..."
"$VENV/bin/python" -m playwright install-deps chromium

echo "دریافت مرورگر Chromium..."
runuser -u rabin-assistant -- env PLAYWRIGHT_BROWSERS_PATH="$BROWSERS" \
  "$VENV/bin/python" -m playwright install chromium

echo "دریافت ابزار بررسی صفحه رابین..."
curl -fsSL "$PY_URL" -o "$BROWSER_DIR/browser-preflight.py"
chown rabin-assistant:rabin-assistant "$BROWSER_DIR/browser-preflight.py"
chmod 0644 "$BROWSER_DIR/browser-preflight.py"

echo
echo "آماده‌سازی تمام شد. هنوز هیچ ورود یا سفارشی انجام نشده است."
echo "برای آزمایش فقط‌خواندنی، این دستور را اجرا کن:"
echo "runuser -u rabin-assistant -- env PLAYWRIGHT_BROWSERS_PATH=$BROWSERS \\"
echo "  RABIN_BROWSER_DIAG_PATH=$DATA_DIR/browser-preflight.png \\"
echo "  $VENV/bin/python $BROWSER_DIR/browser-preflight.py"
echo
echo "این مرحله به سرویس فعلی، پورت‌ها، تنظیمات و سفارش‌های ذخیره‌شده دست نمی‌زند."
