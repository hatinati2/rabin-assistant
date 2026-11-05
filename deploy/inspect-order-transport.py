#!/usr/bin/env python3
"""Read-only inspection of public Rabin frontend transport/auth wiring."""
import json
import re
from urllib.parse import urlsplit, urlunsplit

TARGET = "https://trader.rabin.ir/"
PATTERNS = [
    ("api_origin", r"https?://api\\.rabin\\.ir(?:/api)?"),
    ("base_url", r"\\bbaseURL\\s*[:=]"),
    ("axios_create", r"\\baxios\\.create\\s*\\("),
    ("request_interceptor", r"\\.interceptors\\.request\\.use"),
    ("response_interceptor", r"\\.interceptors\\.response\\.use"),
    ("authorization_header", r"Authorization\\s*[:=]"),
    ("bearer_literal", r"Bearer\\s"),
    ("access_token_reference", r"accessToken"),
    ("token_storage_reference", r"localStorage\\.(?:getItem|setItem)\\s*\\("),
    ("order_endpoint", r"[\\\"']\\/order[\\\"']"),
    ("order_side_payload", r"orderSide\\s*:"),
    ("validity_type_reference", r"validityType\\s*:"),
]

def clean_url(raw):
    p = urlsplit(raw)
    return urlunsplit((p.scheme, p.netloc, p.path, "", ""))

def main():
    try:
        from playwright.sync_api import sync_playwright
    except Exception as exc:
        print(json.dumps({"ok": False, "stage": "import", "error": str(exc)}))
        return 2

    result = {
        "ok": False,
        "purpose": "Inspect public frontend JavaScript for normal-order API transport and auth wiring.",
        "safety": "No login, credentials, browser storage, broker API calls, or order actions.",
        "bundles": [],
        "matches": [],
        "errors": [],
    }

    try:
        with sync_playwright() as p:
            browser = p.chromium.launch(headless=True)
            try:
                context = browser.new_context(locale="fa-IR", timezone_id="Asia/Tehran")
                page = context.new_page()
                response = page.goto(TARGET, wait_until="domcontentloaded", timeout=60000)
                page.wait_for_timeout(3000)
                urls = page.evaluate("""() => [...new Set([
                  ...[...document.scripts].map(s => s.src).filter(Boolean),
                  ...performance.getEntriesByType('resource')
                    .filter(e => /\\.m?js(?:[?#]|$)/i.test(e.name)).map(e => e.name)
                ])].map(raw => {
                  try {
                    const u = new URL(raw, location.href);
                    return u.origin === location.origin ? u.origin + u.pathname : '';
                  } catch (_) { return ''; }
                }).filter(Boolean).slice(0, 100)""")

                for raw_url in urls:
                    try:
                        js_response = context.request.get(raw_url, timeout=25000)
                        if not js_response.ok:
                            continue
                        source = js_response.text()
                        if len(source) > 15000000:
                            continue
                        bundle_url = clean_url(raw_url)
                        result["bundles"].append({"url": bundle_url, "bytes": len(source)})
                        for label, pattern in PATTERNS:
                            for match in re.finditer(pattern, source, re.I):
                                left = max(0, match.start() - 900)
                                right = min(len(source), match.end() + 1100)
                                snippet = re.sub(r"\\s+", " ", source[left:right])
                                result["matches"].append({
                                    "bundle": bundle_url,
                                    "label": label,
                                    "offset": match.start(),
                                    "snippet": snippet[:1800],
                                })
                                if len(result["matches"]) >= 40:
                                    break
                            if len(result["matches"]) >= 40:
                                break
                        if len(result["matches"]) >= 40:
                            break
                    except Exception as exc:
                        result["errors"].append({"url": clean_url(raw_url), "error": str(exc)})

                result.update({
                    "ok": True,
                    "stage": "public_transport_inspected",
                    "http_status": response.status if response else None,
                    "bundle_count": len(result["bundles"]),
                    "match_count": len(result["matches"]),
                })
                context.close()
            finally:
                browser.close()
    except Exception as exc:
        result.update({"ok": False, "stage": "inspection_error", "error": str(exc)})

    print(json.dumps(result, ensure_ascii=False, indent=2), flush=True)
    return 0 if result["ok"] else 1

if __name__ == "__main__":
    raise SystemExit(main())
