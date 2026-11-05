#!/usr/bin/env python3
"""Read-only Rabin login-page probe.

Opens the public trading page, then opens the login UI only.
It never types credentials, submits a login form, or places an order.
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
      const forms = [...document.forms].filter(visible).map(form => ({
        method: (form.method || 'get').toUpperCase(),
        action: form.action || '',
        fields: [...form.querySelectorAll('input,textarea,select')].filter(visible)
          .map(el => ({
            tag: el.tagName.toLowerCase(),
            type: el.getAttribute('type') || '',
            name: el.getAttribute('name') || '',
            id: el.id || '',
            placeholder: el.getAttribute('placeholder') || '',
            ariaLabel: el.getAttribute('aria-label') || ''
          }))
      }));
      const dialogs = [...document.querySelectorAll(
        '[role="dialog"],dialog,[aria-modal="true"]'
      )].filter(visible).map(el => (el.innerText || '').trim().slice(0, 1200));
      return {
        title: document.title,
        url: location.href,
        readyState: document.readyState,
        bodyText: (document.body?.innerText || '').slice(0, 3600),
        controls,
        forms,
        dialogs
      };
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
        "note": "Read-only login UI probe: opens login UI only; no credentials typed, no login submitted, no order placed.",
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

            login_button = page.get_by_role(
                "button", name="ورود به حساب کاربری", exact=True
            ).first
            if login_button.count() == 0:
                login_button = page.get_by_role("button", name="ورود", exact=True).first

            clicked = False
            if login_button.count() > 0:
                try:
                    login_button.click(timeout=5000)
                    page.wait_for_timeout(4000)
                    clicked = True
                except Exception as exc:
                    result["open_login_error"] = str(exc)

            login_ui = collect_page(page)
            result.update({
                "ok": True,
                "stage": "login_ui_inspected" if clicked else "login_button_not_found",
                "http_status": response.status if response else None,
                "initial_page": initial,
                "login_button_clicked": clicked,
                "login_page": login_ui,
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
            result.update({
                "stage": "navigation_or_browser_error",
                "error": str(exc),
            })
            emit(result)
            return 1
        finally:
            if browser is not None:
                browser.close()

if __name__ == "__main__":
    raise SystemExit(main())
