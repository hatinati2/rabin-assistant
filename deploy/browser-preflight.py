#!/usr/bin/env python3
"""Read-only Rabin login implementation probe.

Opens the public page and login panel, inspects field/button metadata, and
searches publicly served JavaScript bundles for login-related route clues.
It never types credentials, submits login, or places an order.
"""
import json
import os
import re
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit

TARGET = "https://trader.rabin.ir/"
SCREENSHOT = Path(os.environ.get(
    "RABIN_BROWSER_DIAG_PATH",
    "/var/lib/rabin-assistant/browser-preflight.png",
))
PATTERNS = ("userName", "login-btn", "password", "login", "auth")

def emit(data):
    print(json.dumps(data, ensure_ascii=False, indent=2), flush=True)

def safe_url(raw):
    p = urlsplit(raw)
    return urlunsplit((p.scheme, p.netloc, p.path, "", ""))

def inspect_login(page):
    return page.evaluate("""() => {
      const visible = el => {
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
        tag: el.tagName.toLowerCase(), type: el.type || '',
        name: el.name || '', id: el.id || '',
        autocomplete: el.autocomplete || '',
        maxLength: el.maxLength, required: !!el.required
      }));
      const form = (pass && pass.closest('form')) || (user && user.closest('form'));
      const submit = [...document.querySelectorAll('button')].find(el =>
        visible(el) && (el.innerText || '').trim() === 'ورود' &&
        el.classList.contains('login-btn')
      );
      const chain = el => {
        const out = [];
        for (let n = el, i = 0; n && i < 6; n = n.parentElement, i++) {
          out.push({
            tag: n.tagName.toLowerCase(),
            id: n.id || '',
            className: String(n.className || '').slice(0, 180)
          });
        }
        return out;
      };
      const scriptURLs = [...new Set([
        ...[...document.scripts].map(s => s.src).filter(Boolean),
        ...performance.getEntriesByType('resource')
          .filter(e => e.initiatorType === 'script' || /\\.m?js(?:[?#]|$)/i.test(e.name))
          .map(e => e.name)
      ])].map(raw => {
        try {
          const u = new URL(raw, location.href);
          return u.origin + u.pathname;
        } catch (_) { return ''; }
      }).filter(Boolean).slice(0, 80);
      return {
        url: location.href,
        fields,
        form: form ? {
          method: (form.method || 'get').toUpperCase(),
          action: form.action || '',
          id: form.id || '',
          className: String(form.className || ''),
          ancestors: chain(form)
        } : null,
        loginButton: submit ? {
          text: (submit.innerText || '').trim(),
          type: submit.type || '',
          className: String(submit.className || ''),
          disabled: !!submit.disabled,
          ancestors: chain(submit)
        } : null,
        scriptURLs,
        securityText: (document.body?.innerText || '').split('\\n')
          .map(x => x.trim()).filter(x => x &&
            /کلید امن|صفحه کلید مجازی|رمز عبور|نام کاربری|کد ملی|captcha|کپچا/i.test(x))
          .slice(0, 30)
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
        "note": "Read-only inspection of public login UI and public JS only. No credentials typed, no login submitted, no order placed.",
    }
    with sync_playwright() as p:
        browser = None
        try:
            browser = p.chromium.launch(headless=True)
            context = browser.new_context(locale="fa-IR", timezone_id="Asia/Tehran",
                                          viewport={"width": 1440, "height": 1000})
            page = context.new_page()
            response = page.goto(TARGET, wait_until="domcontentloaded", timeout=60000)
            page.wait_for_timeout(7000)
            click_info = page.evaluate("""() => {
              const norm = s => (s || '').replace(/\\s+/g, ' ').trim();
              const el = [...document.querySelectorAll('button,[role="button"]')]
                .find(x => norm(x.innerText) === 'ورود به حساب کاربری')
                || [...document.querySelectorAll('button,[role="button"]')]
                .find(x => norm(x.innerText) === 'ورود');
              if (!el) return {found: false};
              el.scrollIntoView({block: 'center', inline: 'center', behavior: 'instant'});
              el.click();
              return {found: true, text: norm(el.innerText)};
            }""")
            if click_info.get("found"):
                page.wait_for_timeout(3000)

            details = inspect_login(page)
            js_matches = []
            for script_url in details.get("scriptURLs", []):
                if len(js_matches) >= 35:
                    break
                try:
                    response_js = context.request.get(script_url, timeout=20000)
                    if not response_js.ok:
                        continue
                    source = response_js.text()
                    for pattern in PATTERNS:
                        start = 0
                        found = 0
                        while found < 3 and len(js_matches) < 35:
                            idx = source.find(pattern, start)
                            if idx < 0:
                                break
                            lo, hi = max(0, idx - 180), min(len(source), idx + 260)
                            snippet = re.sub(r"\\s+", " ", source[lo:hi])
                            js_matches.append({
                                "script": script_url,
                                "pattern": pattern,
                                "snippet": snippet[:500]
                            })
                            start = idx + len(pattern)
                            found += 1
                except Exception:
                    continue

            result.update({
                "ok": True,
                "stage": "login_code_inspected" if click_info.get("found") else "login_button_not_found",
                "http_status": response.status if response else None,
                "login_click": click_info,
                "login_details": details,
                "public_js_matches": js_matches,
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
