#!/usr/bin/env python3
"""Read-only extraction of public Rabin normal-order frontend code contexts."""
import json
import re
from urllib.parse import urlsplit, urlunsplit

TARGET = "https://trader.rabin.ir/"
PATTERNS = [
    ("normal_order_path", r"""["']/order["']""", 12),
    ("preorder_path_for_comparison", r"""["']/pre-order["']""", 5),
    ("normal_order_callback", r"\border\s*:\s*[A-Za-z_$][\w$]*", 12),
    ("preorder_callback", r"\bpreOrder\s*:", 8),
    ("submit_order_symbol", r"\bsubmitOrder\b", 12),
    ("order_side_field", r"\borderSide\s*:", 12),
    ("validity_type_field", r"\bvalidityType\s*:", 12),
    ("order_type_field", r"\borderType\s*:", 8),
    ("instrument_identifier_field", r"\b(?:instrumentIsin|isin|symbol)\s*:", 8),
    ("quantity_field", r"\bquantity\s*:", 8),
    ("price_field", r"\bprice\s*:", 8),
    ("current_orders_route", r"""["']/currentOrders["']""", 5),
]

def clean_url(raw):
    parsed = urlsplit(raw)
    return urlunsplit((parsed.scheme, parsed.netloc, parsed.path, "", ""))

def main():
    try:
        from playwright.sync_api import sync_playwright
    except Exception as exc:
        print(json.dumps({"ok": False, "stage": "import", "error": str(exc)}))
        return 2

    result = {
        "ok": False,
        "purpose": "Extract surrounding public frontend code for the normal order route, callback, and request fields.",
        "safety": "Read-only public JavaScript GETs only; no login, credentials, browser storage, API requests, or order actions.",
        "bundles": [],
        "matches": [],
        "errors": [],
    }

    try:
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(headless=True)
            try:
                context = browser.new_context(locale="fa-IR", timezone_id="Asia/Tehran")
                page = context.new_page()
                response = page.goto(TARGET, wait_until="domcontentloaded", timeout=60000)
                page.wait_for_timeout(2500)
                urls = page.evaluate(r"""() => [...new Set([
                  ...[...document.scripts].map(s => s.src).filter(Boolean),
                  ...performance.getEntriesByType('resource')
                    .filter(e => /\.m?js(?:[?#]|$)/i.test(e.name)).map(e => e.name)
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
                        for label, pattern, limit in PATTERNS:
                            found = 0
                            for match in re.finditer(pattern, source, re.I):
                                left = max(0, match.start() - 1100)
                                right = min(len(source), match.end() + 1400)
                                snippet = re.sub(r"\s+", " ", source[left:right])
                                result["matches"].append({
                                    "label": label,
                                    "bundle": bundle_url,
                                    "offset": match.start(),
                                    "snippet": snippet[:2600],
                                })
                                found += 1
                                if found >= limit:
                                    break
                        if len(result["matches"]) >= 100:
                            break
                    except Exception as exc:
                        result["errors"].append({"url": clean_url(raw_url), "error": str(exc)})

                result.update({
                    "ok": True,
                    "stage": "normal_order_contexts_extracted",
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
