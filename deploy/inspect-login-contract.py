#!/usr/bin/env python3
"""Read-only inspection of public Rabin login and second-factor frontend code."""
import json
import re
from urllib.parse import urlsplit, urlunsplit

TARGET = "https://trader.rabin.ir/"
TERMS = [
    ("login_endpoint", r"authentication/login", 3600),
    ("alternative_factor", r"alternativeFactor", 2200),
    ("two_factor", r"twoFactor|two-factor|two_factor", 2200),
    ("google_authenticator", r"google.?authenticator|authenticator", 1800),
    ("verification_code", r"verification.?code|validation.?code|otp", 1800),
    ("sms_or_email", r"send.?sms|send.?email|sms.?code|email.?code", 1600),
]
MAX_BUNDLES = 120
MAX_MATCHES = 18
MAX_SOURCE_BYTES = 15_000_000

def clean_url(raw):
    parsed = urlsplit(raw)
    return urlunsplit((parsed.scheme, parsed.netloc, parsed.path, "", ""))

def compact(source, start, end, limit):
    value = re.sub(r"\s+", " ", source[max(0, start):min(len(source), end)])
    return value[:limit]

def main():
    try:
        from playwright.sync_api import sync_playwright
    except Exception as exc:
        print(json.dumps({"ok": False, "stage": "import", "error": str(exc)}))
        return 2

    result = {
        "ok": False,
        "purpose": "Inspect public frontend source for the Rabin login payload and second-factor flow.",
        "safety": "Public JavaScript GETs only. Does not log in, submit credentials, inspect browser storage/cookies, or call broker APIs.",
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
                }).filter(Boolean).slice(0, 120)""")

                for raw_url in urls[:MAX_BUNDLES]:
                    try:
                        js_response = context.request.get(raw_url, timeout=25000)
                        if not js_response.ok:
                            continue
                        source = js_response.text()
                        if len(source) > MAX_SOURCE_BYTES:
                            continue
                        bundle_url = clean_url(raw_url)
                        result["bundles"].append({"url": bundle_url, "bytes": len(source)})

                        for label, pattern, radius in TERMS:
                            for match in re.finditer(pattern, source, re.I):
                                if len(result["matches"]) >= MAX_MATCHES:
                                    break
                                result["matches"].append({
                                    "label": label,
                                    "bundle": bundle_url,
                                    "offset": match.start(),
                                    "snippet": compact(source, match.start() - radius, match.end() + radius, 5200),
                                })
                            if len(result["matches"]) >= MAX_MATCHES:
                                break
                        if len(result["matches"]) >= MAX_MATCHES:
                            break
                    except Exception as exc:
                        result["errors"].append({"url": clean_url(raw_url), "error": str(exc)})

                result.update({
                    "ok": True,
                    "stage": "public_login_contract_inspected",
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
