# Rabin Assistant

پنل سبک فارسی برای تعریف و زمان‌بندی سفارش‌های خرید و فروش، با Go و بدون وابستگی بیرونی.

## نصب تک‌خطی روی Ubuntu

چون مخزن خصوصی است، سرور برای دریافت سورس به یک Fine-grained Personal Access Token فقط‌خواندنی نیاز دارد. این توکن را در GitHub بسازید:
- Settings → Developer settings → Personal access tokens → Fine-grained tokens
- Resource owner: `hatinati2`
- Repository access: فقط `rabin-assistant`
- Repository permissions: `Contents: Read-only` (دسترسی Metadata به‌صورت پیش‌فرض)
- برای امنیت بیشتر، تاریخ انقضای کوتاه انتخاب کنید.

سپس با کاربر root، فقط همین یک خط را روی سرور Paste کنید. اسکریپت توکن را بدون نمایش می‌پرسد، آن را موقتاً در فایل دسترسی‌محدود نگه می‌دارد، فایل مخزن خصوصی را دریافت می‌کند و در پایان فایل موقت را پاک می‌کند. توکن در دستور، آرگومان پردازش یا Git ذخیره نمی‌شود.

```bash
bash -lc 'set -Eeuo pipefail; umask 077; if ! command -v curl >/dev/null 2>&1 || ! command -v tar >/dev/null 2>&1; then apt-get update; apt-get install -y curl tar; fi; read -r -s -p "GitHub read-only token: " GH_TOKEN; printf "\n"; [ -n "$GH_TOKEN" ] || { echo "Token is required"; exit 1; }; tmp="$(mktemp -d)"; trap '\''rm -rf "$tmp"; unset GH_TOKEN'\'' EXIT; printf "header = \"Authorization: Bearer %s\"\nheader = \"Accept: application/vnd.github+json\"\n" "$GH_TOKEN" > "$tmp/api.conf"; unset GH_TOKEN; curl -fsS --config "$tmp/api.conf" -D "$tmp/headers" -o /dev/null "https://api.github.com/repos/hatinati2/rabin-assistant/tarball/main"; archive_url="$(awk '\''tolower($1)=="location:" { sub(/\r$/, ""); $1=""; sub(/^[[:space:]]+/, ""); url=$0 } END { print url }'\'' "$tmp/headers")"; [ -n "$archive_url" ] || { echo "GitHub archive URL not found; check token permissions."; exit 1; }; curl -fsSL "$archive_url" -o "$tmp/repo.tar.gz"; mkdir "$tmp/repo"; tar -xzf "$tmp/repo.tar.gz" -C "$tmp/repo" --strip-components=1; bash "$tmp/repo/deploy/install.sh"'
```

> توکن باید فقط دسترسی خواندن Contents همین مخزن را داشته باشد. آن را در چت، فایل پروژه یا ترمینال با echo چاپ نکنید.

نصب‌کننده فایل اجرایی را در `/opt/rabin-assistant` می‌سازد، داده‌ها را در `/var/lib/rabin-assistant` نگه می‌دارد و سرویس systemd را فعال می‌کند. اگر Go نصب نباشد، نصب‌کننده متوقف می‌شود و باید Go 1.23 یا جدیدتر نصب شود.

اطلاعات ورود اولیه:
```bash
cat /root/rabin-assistant-credentials.txt
```

برای دسترسی امن از رایانه:
```bash
ssh -L 8098:127.0.0.1:8098 root@SERVER_IP
```
سپس `http://127.0.0.1:8098` را باز کنید. سرویس فقط روی localhost گوش می‌دهد؛ پورت عمومی باز نکنید.

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

## وضعیت و محدودیت نسخه فعلی

این نسخه سفارش‌ها را در فایل JSON محلی ذخیره می‌کند و در زمان تعیین‌شده وضعیت آن‌ها را به «آماده بررسی» تغییر می‌دهد. **هیچ ورود خودکاری به کارگزاری رابین و هیچ سفارش واقعی ارسال، ویرایش یا لغو نمی‌شود.** اتصال کارگزاری باید پس از دریافت روش اتصال رسمی/مورد تأیید و مستندات فنی جداگانه توسعه داده شود.

## امنیت

- رمز ورود تصادفی تولید و در فایل با دسترسی محدود ذخیره می‌شود.
- فایل داده، تنظیمات، کلیدها و اطلاعات ورود نباید وارد Git شوند.
- برای دسترسی به پنل از SSH tunnel استفاده کنید.
- توکن GitHub فقط برای دریافت سورس است و باید فقط دسترسی `Contents: Read-only` به همین مخزن داشته باشد.
