# Rabin Assistant

پنل سبک فارسی برای تعریف و زمان‌بندی سفارش‌های خرید و فروش، با Go و بدون وابستگی بیرونی.

## نصب با یک دستور روی Ubuntu

فقط همین یک دستور را با کاربر root روی سرور Paste کنید. هنگام اجرا، اسکریپت توکن GitHub را به‌صورت مخفی می‌پرسد؛ سپس سورس مخزن خصوصی را دانلود، پیش‌نیازهای لازم را نصب، برنامه را کامپایل و سرویس را راه‌اندازی می‌کند:

```bash
bash -lc 'set -Eeuo pipefail; umask 077; if ! command -v curl >/dev/null 2>&1 || ! command -v tar >/dev/null 2>&1; then apt-get update; apt-get install -y curl tar; fi; read -r -s -p "GitHub read-only token: " T; printf "\n"; [ -n "$T" ] || { echo "توکن لازم است."; exit 1; }; D="$(mktemp -d)"; trap '\''rm -rf "$D"; unset T'\'' EXIT; printf "header = \"Authorization: Bearer %s\"\n" "$T" > "$D/curl.conf"; unset T; curl -fsS --config "$D/curl.conf" -D "$D/headers" -o /dev/null https://api.github.com/repos/hatinati2/rabin-assistant/tarball/main; U="$(awk '\''tolower($1)=="location:" {gsub("\r",""); u=$2} END {print u}'\'' "$D/headers")"; [ -n "$U" ] || { echo "دریافت لینک دانلود ناموفق بود؛ مجوز توکن را بررسی کنید."; exit 1; }; curl -fsSL "$U" | tar -xz --strip-components=1 -C "$D"; bash "$D/deploy/install.sh"'
```

### ساخت توکن فقط‌خواندنی GitHub

در GitHub از مسیر Settings → Developer settings → Personal access tokens → Fine-grained tokens یک توکن بسازید:

- Resource owner: `hatinati2`
- Repository access: فقط `rabin-assistant`
- Repository permissions: `Contents: Read-only`
- گزینه دسترسی نوشتن لازم نیست.
- بهتر است برای توکن تاریخ انقضای کوتاه انتخاب کنید.

توکن هنگام نصب به شکل مخفی دریافت و فقط موقتاً در فایل با مجوز محدود نگهداری می‌شود؛ در کد، آرگومان پردازش یا Git ذخیره نمی‌شود.

## چه کارهایی نصب‌کننده انجام می‌دهد؟

1. بررسی Ubuntu، دسترسی root و systemd
2. نصب Go در صورت نبودن آن (نسخه 1.23 یا جدیدتر)
3. ساخت کاربر سیستمی اختصاصی
4. ساخت تنظیمات و رمز تصادفی پنل
5. کامپایل برنامه Go
6. نصب و فعال‌سازی سرویس systemd
7. محدود کردن سرویس به localhost

اطلاعات ورود اولیه:
```bash
cat /root/rabin-assistant-credentials.txt
```

برای باز کردن پنل از مرورگر:
`http://SERVER_IP:8098`

در صورت نیاز، اجازه دسترسی به TCP پورت 8098 را در فایروال سیستم‌عامل و پنل ارائه‌دهنده سرور تنظیم کنید.

## مدیریت سرویس

```bash
systemctl status rabin-assistant
journalctl -u rabin-assistant -f
systemctl restart rabin-assistant
```

## تنظیمات

فایل محیطی: `/etc/rabin-assistant.env`

- `RABIN_ADMIN_USER`
- `RABIN_ADMIN_PASS`
- `RABIN_ADDR` (پیش‌فرض `127.0.0.1:8098`)
- `RABIN_DATA_DIR` (پیش‌فرض `/var/lib/rabin-assistant`)

منطقه زمانی برنامه `Asia/Tehran` است.

## وضعیت نسخه فعلی

این نسخه سفارش‌ها را در فایل JSON محلی ذخیره می‌کند و در زمان تعیین‌شده وضعیت آن‌ها را به «آماده بررسی» تغییر می‌دهد. **هیچ ورود خودکاری به کارگزاری رابین و هیچ سفارش واقعی ارسال، ویرایش یا لغو نمی‌شود.** اتصال کارگزاری باید پس از دریافت روش اتصال رسمی/مورد تأیید و مستندات فنی جداگانه توسعه داده شود.

## امنیت

- رمز ورود تصادفی تولید و در فایل با دسترسی محدود ذخیره می‌شود.
- فایل داده، تنظیمات، کلیدها و اطلاعات ورود نباید وارد Git شوند.
- آدرس پیش‌فرض پنل `0.0.0.0:8098` است و از طریق IP سرور روی پورت 8098 باز می‌شود.
- توکن GitHub فقط باید دسترسی خواندن همین مخزن را داشته باشد.
