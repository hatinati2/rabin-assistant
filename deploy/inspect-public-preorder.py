#!/usr/bin/env python3
"""Read-only inspection of public Rabin pre-order UI code.

Loads the public trading page and its public JavaScript bundles only.
Does not log in, call authentication/order endpoints, type credentials, or submit orders.
"""
import json
import re
from urllib.parse import urlsplit, urlunsplit

TARGET = "https://trader.rabin.ir/"
TERMS = (
    "sendGroupPreOrder",
    "sendAllPreOrder",
    "getAllPreOrder",
    "preOrder:",
    '"pre-order/send"',
    "'/pre-order/send'",
    '"/pre-order"',
    "'/pre-order'",
    "pre-order",
)
MAX_MATCHES = 45
CONTEXT = 1100

def safe_url(raw):
    p = urlsplit(raw)
    return urlunsplit((p.scheme, p.netloc, p.path, "", ""))

def main():
    try:
        from playwright.sync_api import sync_playwright
    except Exception as exc:
        print(json.dumps({"ok": False, "stage": "import", "error": str(exc)}))
        return 2

    output = {
        "ok": False,
        "stage": "starting",
        "note": "Public static JavaScript inspection only. No login, authentication calls, order API calls, or order submission.",
        "bundles": [],
        "matches": [],
    }
    with sync_playwright() as p:
        browser = None
        try:
            browser = p.chromium.launch(headless=True)
            context = browser.new_context(locale="fa-IR", timezone_id="Asia/Tehran")
            page = context.new_page()
            response = page.goto(TARGET, wait_until="domcontentloaded", timeout=60000)
            page.wait_for_timeout(6000)
            urls = page.evaluate("""() => [...new Set([
              ...[...document.scripts].map(s => s.src).filter(Boolean),
              ...performance.getEntriesByType('resource')
                .filter(e => /\\.m?js(?:[?#]|$)/i.test(e.name))
                .map(e => e.name)
            ])].map(raw => {
              try { const u = new URL(raw, location.href); return u.origin + u.pathname; }
              catch (_) { return ''; }
            }).filter(u => u && new URL(u).origin === location.origin).slice(0, 100)""")

            for url in urls:
                try:
                    r = context.request.get(url, timeout=25000)
                    if not r.ok:
                        continue
                    source = r.text()
                    if len(source) > 12_000_000:
                        continue
                    output["bundles"].append({"url": safe_url(url), "bytes": len(source)})
                    for term in TERMS:
                        pos = 0
                        while len(output["matches"]) < MAX_MATCHES:
                            idx = source.find(term, pos)
                            if idx < 0:
                                break
                            left = max(0, idx - CONTEXT)
                            right = min(len(source), idx + len(term) + CONTEXT)
                            snippet = re.sub(r"\\s+", " ", source[left:right])
                            output["matches"].append({
                                "bundle": safe_url(url),
                                "term": term,
                                "offset": idx,
                                "snippet": snippet[:2400],
                            })
                            pos = idx + len(term)
                        if len(output["matches"]) >= MAX_MATCHES:
                            break
                except Exception:
                    continue

            output.update({
                "ok": True,
                "stage": "public_order_ui_code_inspected",
                "http_status": response.status if response else None,
                "bundle_count": len(output["bundles"]),
                "match_count": len(output["matches"]),
            })
            print(json.dumps(output, ensure_ascii=False, indent=2), flush=True)
            context.close()
            return 0
        except Exception as exc:
            output.update({"stage": "browser_error", "error": str(exc)})
            print(json.dumps(output, ensure_ascii=False, indent=2), flush=True)
            return 1
        finally:
            if browser is not None:
                browser.close()

if __name__ == "__main__":
    raise SystemExit(main())
