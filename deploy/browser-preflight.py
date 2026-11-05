#!/usr/bin/env python3
"""Read-only connectivity and DOM probe for trader.rabin.ir.

This probe never signs in, types into fields, or clicks buttons.
It only reads the public/login page and writes a diagnostic screenshot.
"""
import json
import os
import sys
from pathlib import Path

TARGET = "https://trader.rabin.ir/"
SCREENSHOT = Path(os.environ.get(
    "RABIN_BROWSER_DIAG_PATH",
    "/var/lib/rabin-assistant/browser-preflight.png",
))

def emit(data):
    print(json.dumps(data, ensure_ascii=False, indent=2), flush=True)

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
        "note": "Read-only check: no login and no order submission.",
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

            details = page.evaluate("""() => {
              const visible = (el) => {
                const r = el.getBoundingClientRect();
                const s = getComputedStyle(el);
                return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && s.display !== 'none';
              };
              const controls = [...document.querySelectorAll(
                'input,textarea,select,button,[role="button"]'
              )].filter(visible).slice(0, 100).map(el => {
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
                  text: (el.innerText || el.value || '').trim().slice(0, 120),
                  disabled: !!el.disabled
                };
              });
              return {
                title: document.title,
                url: location.href,
                readyState: document.readyState,
                bodyText: (document.body?.innerText || '').slice(0, 3000),
                controls
              };
            }""")

            result.update({
                "ok": True,
                "stage": "page_loaded",
                "http_status": response.status if response else None,
                **details,
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
