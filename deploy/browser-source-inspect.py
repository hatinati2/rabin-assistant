#!/usr/bin/env python3
"""Read-only inspection of Rabin's public login and order-related frontend code.

This diagnostic opens the public login panel and reads publicly served JS bundles.
It never enters credentials, submits login, sends API requests, or places orders.
"""
import json
import os
import re
from pathlib import Path
from urllib.parse import urlsplit

TARGET = "https://trader.rabin.ir/"
SCREENSHOT = Path(os.environ.get(
    "RABIN_BROWSER_DIAG_PATH",
    "/var/lib/rabin-assistant/browser-preflight.png",
))
NEEDLES = [
    "login-btn",
    "checkUser",
    "hasAvoidToUseVirtualKeyboardConfirmed",
    "googleAuth",
    "emailOtp",
    "securityCode",
    "security-code",
    "accessToken",
    "Authorization",
    "authentication",
    "submitOrder",
    "newOrder",
    "buyOrder",
    "sellOrder",
    "marketOrder",
    "limitOrder",
]
PATH_WORDS = re.compile(
    r"auth|login|token|order|trade|buy|sell|portfolio|request|security|otp",
    re.I,
)

def emit(obj):
    print(json.dumps(obj, ensure_ascii=False, indent=2), flush=True)

def safe_script_url(raw):
    try:
        u = urlsplit(raw)
        return u.scheme + "://" + u.netloc + u.path
    except Exception:
        return ""

def inspect_login(page):
    return page.evaluate("""() => {
      const visible = el => {
        const r = el.getBoundingClientRect();
        const s = getComputedStyle(el);
        return r.width > 0 && r.height > 0 &&
          s.visibility !== 'hidden' && s.display !== 'none';
      };
      const scriptURLs = [...new Set([
        ...[...document.scripts].map(s => s.src).filter(Boolean),
        ...performance.getEntriesByType('resource')
          .filter(e => e.initiatorType === 'script' || /\\.m?js(?:[?#]|$)/i.test(e.name))
          .map(e => e.name)
      ])].map(raw => {
        try { const u = new URL(raw, location.href); return u.origin + u.pathname; }
        catch (_) { return ''; }
      }).filter(Boolean).slice(0, 100);
      const user = [...document.querySelectorAll('input')].find(el =>
        visible(el) && el.name === 'userName');
      const pass = [...document.querySelectorAll('input')].find(el =>
        visible(el) && el.name === 'password');
      const loginButton = [...document.querySelectorAll('button')].find(el =>
        visible(el) && (el.innerText || '').trim() === 'ورود' &&
        el.classList.contains('login-btn'));
      const checkbox = [...document.querySelectorAll('input[type=checkbox]')].filter(visible)
        .map(el => ({id: el.id || '', name: el.name || '',
          checked: !!el.checked, label: el.closest('label')?.innerText?.trim() || ''}));
      return {
        url: location.href,
        fields: [user, pass].filter(Boolean).map(el => ({
          type: el.type || '', name: el.name || '', autocomplete: el.autocomplete || '',
          required: !!el.required
        })),
        loginButton: loginButton ? {
          text: (loginButton.innerText || '').trim(), type: loginButton.type || '',
          className: String(loginButton.className || '')
        } : null,
        checkboxes: checkbox,
        scriptURLs,
        visibleSecurityText: (document.body?.innerText || '').split('\\n')
          .map(s => s.trim()).filter(s => s &&
            /کلید امن|صفحه کلید مجازی|رمز عبور|نام کاربری|کد ملی|کد امنیتی|captcha|کپچا/i.test(s))
          .slice(0, 25)
      };
    }""")

def extract_matches(source, script_url):
    contexts = []
    seen = set()
    for needle in NEEDLES:
        start = 0
        count = 0
        while count < 3:
            idx = source.lower().find(needle.lower(), start)
            if idx < 0:
                break
            lo = max(0, idx - 420)
            hi = min(len(source), idx + 620)
            snippet = re.sub(r"\s+", " ", source[lo:hi])
            key = (needle, snippet[:180])
            if key not in seen:
                contexts.append({
                    "script": script_url,
                    "needle": needle,
                    "context": snippet[:1100]
                })
                seen.add(key)
            start = idx + len(needle)
            count += 1
            if len(contexts) >= 32:
                return contexts
    return contexts

def extract_route_strings(source, script_url):
    found = []
    seen = set()
    # Candidate path strings only; no request is made to these paths.
    for match in re.finditer(r"""["'](/[^"'\\\s]{2,120})["']""", source):
        path = match.group(1)
        if PATH_WORDS.search(path):
            if path not in seen:
                found.append({"script": script_url, "path": path})
                seen.add(path)
            if len(found) >= 80:
                break
    return found

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
        "note": "Read-only public-source inspection only. No credentials, login submission, API calls, or order placement.",
    }
    with sync_playwright() as p:
        browser = None
        try:
            browser = p.chromium.launch(headless=True)
            context = browser.new_context(
                locale="fa-IR", timezone_id="Asia/Tehran",
                viewport={"width": 1440, "height": 1000},
            )
            page = context.new_page()
            response = page.goto(TARGET, wait_until="domcontentloaded", timeout=60000)
            page.wait_for_timeout(6000)
            click_info = page.evaluate("""() => {
              const norm = s => (s || '').replace(/\\s+/g, ' ').trim();
              const el = [...document.querySelectorAll('button,[role="button"]')]
                .find(x => norm(x.innerText) === 'ورود به حساب کاربری');
              if (!el) return {found: false};
              el.scrollIntoView({block: 'center', inline: 'center', behavior: 'instant'});
              el.click();
              return {found: true, text: norm(el.innerText)};
            }""")
            if click_info.get("found"):
                page.wait_for_timeout(2500)
            details = inspect_login(page)

            js_contexts = []
            route_strings = []
            scan_summary = []
            for script_url in details.get("scriptURLs", []):
                try:
                    r = context.request.get(script_url, timeout=20000)
                    if not r.ok:
                        continue
                    source = r.text()
                    if len(source) > 15_000_000:
                        scan_summary.append({"script": script_url, "skipped": "over_15MB"})
                        continue
                    scan_summary.append({
                        "script": script_url,
                        "bytes": len(source),
                    })
                    js_contexts.extend(extract_matches(source, script_url))
                    route_strings.extend(extract_route_strings(source, script_url))
                except Exception as exc:
                    scan_summary.append({
                        "script": script_url,
                        "error": type(exc).__name__,
                    })

            # Deduplicate route strings and prioritize auth/order paths.
            unique_routes = []
            seen_routes = set()
            for item in route_strings:
                key = (item["script"], item["path"])
                if key not in seen_routes:
                    unique_routes.append(item)
                    seen_routes.add(key)

            result.update({
                "ok": True,
                "stage": "public_source_inspected",
                "http_status": response.status if response else None,
                "login_click": click_info,
                "login_ui": details,
                "script_scan": scan_summary,
                "public_js_contexts": js_contexts[:32],
                "route_string_candidates": unique_routes[:80],
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
            result.update({"stage": "browser_error", "error": str(exc)})
            emit(result)
            return 1
        finally:
            if browser is not None:
                browser.close()

if __name__ == "__main__":
    raise SystemExit(main())
