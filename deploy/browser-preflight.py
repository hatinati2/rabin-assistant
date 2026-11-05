#!/usr/bin/env python3
"""Read-only Rabin login UI inspection.

Opens the public page and login panel, then reports field metadata only.
It never types credentials, submits login, or places an order.
"""
import json
import os
from pathlib import Path

TARGET = "https://trader.rabin.ir/"
SCREENSHOT = Path(os.environ.get(
    "RABIN_BROWSER_DIAG_PATH",
    "/var/lib/rabin-assistant/browser-preflight.png",
))

def emit(data):
    print(json.dumps(data, ensure_ascii=False, indent=2), flush=True)

def collect_page(page):
    return page.evaluate("""() => {
      const visible = (el) => {
        const r = el.getBoundingClientRect();
        const s = getComputedStyle(el);
        return r.width > 0 && r.height > 0 &&
          s.visibility !== 'hidden' && s.display !== 'none';
      };
      const controls = [...document.querySelectorAll(
        'input,textarea,select,button,[role="button"]'
      )].filter(visible).slice(0, 120).map(el => {
        const label = el.id
          ? document.querySelector('label[for="' + CSS.escape(el.id) + '"]')
          : null;
        return {
          tag: el.tagName.toLowerCase(),
          type: el.getAttribute('type') || '',
          name: el.getAttribute('name') || '',
          id: el.id || '',
          placeholder: el.getAttribute('placeholder') || '',
          ariaLabel: el.getAttribute('aria-label') || '',
          label: label ? (label.innerText || '').trim() : '',
          text: (el.innerText || '').trim().slice(0, 120),
          disabled: !!el.disabled
        };
      });
      return {
        title: document.title,
        url: location.href,
        readyState: document.readyState,
        bodyText: (document.body?.innerText || '').slice(0, 3600),
        controls
      };
    }""")

def inspect_login_fields(page):
    return page.evaluate("""() => {
      const visible = (el) => {
        const r = el.getBoundingClientRect();
        const s = getComputedStyle(el);
        return r.width > 0 && r.height > 0 &&
          s.visibility !== 'hidden' && s.display !== 'none';
      };
      const user = [...document.querySelectorAll('input')].find(el =>
        visible(el) && el.name === 'userName'
      );
      const pass = [...document.querySelectorAll('input')].find(el =>
        visible(el) && el.name === 'password'
      );
      const fields = [user, pass].filter(Boolean).map(el => ({
        tag: el.tagName.toLowerCase(),
        type: el.type || '',
        name: el.name || '',
        id: el.id || '',
        placeholder: el.placeholder || '',
        autocomplete: el.autocomplete || '',
        inputMode: el.inputMode || '',
        maxLength: el.maxLength,
        required: !!el.required,
        disabled: !!el.disabled
      }));
      const form = (pass && pass.closest('form')) || (user && user.closest('form'));
      const formInfo = form ? {
        method: (form.method || 'get').toUpperCase(),
        action: form.action || '',
        id: form.id || '',
        className: String(form.className || ''),
        fields: [...form.querySelectorAll('input,textarea,select')].map(el => ({
          tag: el.tagName.toLowerCase(),
          type: el.getAttribute('type') || '',
          name: el.getAttribute('name') || '',
          id: el.id || '',
          autocomplete: el.getAttribute('autocomplete') || '',
          required: el.hasAttribute('required')
        })),
        buttons: [...form.querySelectorAll('button,input[type="submit"],input[type="button"]')]
          .filter(visible).map(el => ({
            tag: el.tagName.toLowerCase(),
            type: el.getAttribute('type') || '',
            text: (el.innerText || el.value || '').trim().slice(0, 100),
            className: String(el.className || '')
          }))
      } : null;
      const captcha = [...document.querySelectorAll(
        '[class*="captcha" i],[id*="captcha" i],iframe[src*="captcha" i],img[alt*="captcha" i],canvas'
      )].filter(visible).slice(0, 20).map(el => ({
        tag: el.tagName.toLowerCase(),
        id: el.id || '',
        className: String(el.className || ''),
        alt: el.getAttribute('alt') || '',
        text: (el.innerText || '').trim().slice(0, 120)
      }));
      const securityText = (document.body?.innerText || '')
        .split('\\n')
        .map(x => x.trim())
        .filter(x => x && /کلید امن|صفحه کلید مجازی|رمز عبور|نام کاربری|کد ملی|captcha|کپچا/i.test(x))
        .slice(0, 30);
      return {fields, form: formInfo, captchaHints: captcha, securityText};
    }""")

def main():
    try:
        from playwright.sync_api import sync_playwright
    except Exception as exc:
        emit({"ok": False, "stage": "import_playwright", "error": str(exc)})
        return 2

    result = {
        "ok": False,
        "target": TARGET,
        "stage": "starting",
        "note": "Read-only inspection. No credentials typed, no login submitted, and no order placed.",
    }
    with sync_playwright() as p:
        browser = None
        try:
            browser = p.chromium.launch(headless=True)
            context = browser.new_context(
                locale="fa-IR",
                timezone_id="Asia/Tehran",
                viewport={"width": 1440, "height": 1000},
            )
            page = context.new_page()
            response = page.goto(TARGET, wait_until="domcontentloaded", timeout=60000)
            page.wait_for_timeout(7000)
            initial = collect_page(page)

            click_info = page.evaluate("""() => {
              const norm = s => (s || '').replace(/\\s+/g, ' ').trim();
              const candidates = [...document.querySelectorAll('button,[role="button"]')];
              const el = candidates.find(x =>
                norm(x.innerText) === 'ورود به حساب کاربری'
              ) || candidates.find(x => norm(x.innerText) === 'ورود');
              if (!el) return {found: false};
              el.scrollIntoView({block: 'center', inline: 'center', behavior: 'instant'});
              el.click();
              return {found: true, text: norm(el.innerText)};
            }""")
            if click_info.get("found"):
                page.wait_for_timeout(3500)

            login_ui = collect_page(page)
            login_details = inspect_login_fields(page)
            result.update({
                "ok": True,
                "stage": "login_form_inspected" if login_details.get("fields") else (
                    "login_button_clicked_fields_not_found" if click_info.get("found")
                    else "login_button_not_found"
                ),
                "http_status": response.status if response else None,
                "initial_page": initial,
                "login_click": click_info,
                "login_page": login_ui,
                "login_details": login_details,
            })
            try:
                SCREENSHOT.parent.mkdir(parents=True, exist_ok=True)
                page.screenshot(path=str(SCREENSHOT), full_page=True)
                result["screenshot"] = str(SCREENSHOT)
            except Exception as exc:
                result["screenshot_error"] = str(exc)

            emit(result)
            context.close()
            return 0
        except Exception as exc:
            result.update({"stage": "navigation_or_browser_error", "error": str(exc)})
            emit(result)
            return 1
        finally:
            if browser is not None:
                browser.close()

if __name__ == "__main__":
    raise SystemExit(main())
