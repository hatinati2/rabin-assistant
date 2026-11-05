#!/usr/bin/env python3
"""Read-only inspection of public Rabin frontend transport/auth wiring.

Downloads public same-origin JavaScript only. It does not log in, read browser
storage, call broker API endpoints, or submit/modify/cancel any order.
"""
import json
import re
from urllib.parse import urlsplit, urlunsplit

TARGET = "https://trader.rabin.ir/"
MAX_BUNDLES = 100
MAX_MATCHES = 32
MAX_SNIPPET = 1900

PATTERNS = [
    ("api_origin", r"https?://api\.rabin\.ir(?:/api)?"),
    ("base_url", r"\bbaseURL\s*[:=]"),
    ("axios_create", r"\baxios\.create\s*\("),
    ("request_interceptor", r"\.interceptors\.request\.use"),
    ("response_interceptor", r"\.interceptors\.response\.use"),
    ("authorization_header", r"Authorization\s*[:=]"),
    ("bearer_literal", r"Bearer\s"),
    ("access_token_reference", r"accessToken"),
    ("token_storage_reference", r"localStorage\.(?:getItem|setItem)\s*\("),
    ("order_endpoint", r"[\"']\/order[\"']"),
    ("normal_order_side_payload", r"orderSide\s*:\s*[\"']BUY[\"']"),
    ("validity_type_reference", r"validityType\s*:"),
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

    out = {
        "ok": False,
        "stage": "starting",
        "purpose": "Identify public frontend API transport, auth wiring, and normal-order enum references.",
        "safety": "Read-only public JS inspection only; no login, storage reads, API requests, or order actions.",
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
            page.wait_for_timeout(4000)
            urls = page.evaluate(r"""() => [...new Set([
              ...[...document.scripts].map(s => s.src).filter(Boolean),
              ...performance.getEntriesByType('resource')
                .filter(e => /\.m?js(?:[?#]|$)/i.test(e.name))
                .map(e => e.name)
            ])].map(raw => {
              try {
                const u = new URL(raw, location.href);
                return u.origin === location.origin ? u.origin + u.pathname : '';
              } catch (_) { return ''; }
            }).filter(Boolean).slice(0, 100)""")

            for raw_url in urls[:MAX_BUNDLES]:
                if len(out["matches"]) >= MAX_MATCHES:
                    break
                try:
                    response_js = context.request.get(raw_url, timeout=25000)
                    if not response_js.ok:
                        continue
                    source = response_js.text()
                    if len(source) > 15_000_000:
                        continue
                    bundle = clean_url(raw_url)
                    out["bundles"].append({"url": bundle, "bytes": len(source)})

                    for label, pattern in PATTERNS:
                        for match in re.finditer(pattern, source, re.I):
                            if len(out["matches"]) >= MAX_MATCHES:
                                break
                            left = max(0, match.start() - 1150)
                            right = min(len(source), match.end() + 1450)
                            snippet = re.sub(r"\s+", " ", source[left:right])
                            out["matches"].append({
                                "bundle": bundle,
                                "label": label,
                                "offset": match.start(),
                                "snippet": snippet[:MAX_SNIPPET],
                            })
                        if len(out["matches"]) >= MAX_MATCHES:
                            break

            out.update({
                "ok": True,
                "stage": "public_transport_inspected",
                "http_status": response.status if response else None,
                "bundle_count": len(out["bundles"]),
                "match_count": len(out["matches"]),
            })
            print(json.dumps(out, ensure_ascii=False, indent=2), flush=True)
            context.close()
            return 0
        except Exception as exc:
            out.update({"stage": "browser_error", "error": str(exc)})
            print(json.dumps(out, ensure_ascii=False, indent=2), flush=True)
            return 1
        finally:
            if browser is not None:
                browser.close()

if __name__ == "__main__":
    raise SystemExit(main())
