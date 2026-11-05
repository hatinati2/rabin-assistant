# Rabin Assistant

پنل سبک فارسی برای تعریف و زمان‌بندی سفارش‌های خرید و فروش، با Go و بدون وابستگی بیرونی.

## وضعیت نسخه فعلی

این نسخه سفارش‌ها را در فایل JSON محلی ذخیره می‌کند و در زمان تعیین‌شده وضعیت آن‌ها را به «آماده بررسی» تغییر می‌دهد. **هیچ ورود خودکاری به کارگزاری رابین و هیچ سفارش واقعی ارسال، ویرایش یا لغو نمی‌شود.** اتصال کارگزاری باید پس از دریافت روش اتصال رسمی/مورد تأیید و مستندات فنی جداگانه توسعه داده شود.

## نصب یک‌خطی روی Ubuntu (مخزن خصوصی)

ابتدا روی سرور یک کلید Deploy فقط-خواندنی بسازید:

```bash
install -d -m 700 /root/.ssh
ssh-keygen -t ed25519 -C "rabin-assistant-readonly" -f /root/.ssh/rabin_assistant_github -N ""
cat /root/.ssh/rabin_assistant_github.pub
```

متن کلید عمومی را در GitHub در مسیر زیر اضافه کنید:
**Repository → Settings → Deploy keys → Add deploy key**.
گزینه **Allow write access** را فعال نکنید؛ برای نصب فقط دسترسی خواندن لازم است.

سپس این یک خط را با کاربر root روی سرور اجرا کنید:

```bash
bash -lc 'set -euo pipefail; test -f /root/.ssh/rabin_assistant_github || { echo "ابتدا Deploy Key را بسازید و در GitHub ثبت کنید."; exit 1; }; command -v git >/dev/null 2>&1 || { apt-get update; apt-get install -y git; }; tmp="$(mktemp -d)"; trap '\''rm -rf "$tmp"'\'' EXIT; GIT_SSH_COMMAND="ssh -i /root/.ssh/rabin_assistant_github -o IdentitiesOnly=yes -o BatchMode=yes -o StrictHostKeyChecking=accept-new" git clone --depth 1 git@github.com:hatinati2/rabin-assistant.git "$tmp/repo"; bash "$tmp/repo/install.sh"'
```

اسکریپت فایل اجرایی را در `/opt/rabin-assistant` می‌سازد، داده‌ها را در `/var/lib/rabin-assistant` نگه می‌دارد و سرویس systemd را فعال می‌کند. فایل موقت مخزن پس از نصب پاک می‌شود.

اطلاعات ورود اولیه:
```bash
cat /root/rabin-assistant-credentials.txt
```

از رایانه خود تونل SSH برقرار کنید:
```bash
ssh -L 8098:127.0.0.1:8098 root@SERVER_IP
```

سپس در مرورگر رایانه خود باز کنید:
`http://127.0.0.1:8098`

سرویس فقط روی localhost گوش می‌دهد؛ پورت عمومی جدیدی باز نکنید و آن را مستقیم روی اینترنت منتشر نکنید.

## نصب از سورس موجود

از ریشه پروژه اجرا کنید:

```bash
bash install.sh
```

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

## امنیت

- رمز ورود تصادفی تولید و در فایل با دسترسی محدود ذخیره می‌شود.
- فایل داده، تنظیمات و اطلاعات ورود نباید وارد Git شوند.
- برای دسترسی از SSH tunnel استفاده کنید.
- کلید Deploy فقط-خواندنی است؛ کلید خصوصی را روی سرور نگه دارید و هرگز در GitHub یا چت ارسال نکنید.
- این نرم‌افزار در وضعیت فعلی ابزار زمان‌بندی و آماده‌سازی است، نه ربات ارسال سفارش.
