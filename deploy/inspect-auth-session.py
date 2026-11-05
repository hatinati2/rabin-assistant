#!/usr/bin/env python3
"""Read-only inspection of public Rabin authentication/session lifecycle code."""
import json
import re
from urllib.parse import urlsplit, urlunsplit

TARGET = "https://trader.rabin.ir/"
PATTERNS = [
    ("refresh_token_name", r"\brefreshToken\b|\brefresh_token\b", 6),
    ("refresh_endpoint", r"""["'][^"']*(?:refresh|renew)[^"']*["']""", 8),
    ("refresh_function_name", r"\b(?:refreshAccessToken|renewAccessToken|refreshSession)\b", 6),
    ("token_expiry", r"\b(?:expiresIn|expiresAt|tokenExpiry|expirationTime)\b", 6),
    ("access_token_storage", r"""localStorage\.(?:getItem|setItem|removeItem)\(["']accessToken["']""", 8),
    ("access_token_key_reference", r"""["']accessToken["']""", 8),
    ("active_session_endpoint", r"""["'][^"']*active-session[^"']*["']""", 4),
    ("two_factor_endpoint", r"""["'][^"']*(?:otp|two-factor|google-auth|validation-code)[^"']*["']""", 8),
    ("login_endpoint", r"""["']/authentication/login["']""", 4),
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
        "purpose": "Inspect public frontend code for token persistence, expiry, refresh, and MFA flow.",
        "safety": "Public JavaScript GETs only; does not log in, read browser storage, access credentials, or call broker APIs.",
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
                            count = 0
                            for match in re.finditer(pattern, source, re.I):
                                left = max(0, match.start() - 950)
                                right = min(len(source), match.end() + 1350)
                                snippet = re.sub(r"\s+", " ", source[left:right])
                                result["matches"].append({
                                    "label": label,
                                    "bundle": bundle_url,
                                    "offset": match.start(),
                                    "snippet": snippet[:2500],
                                })
                                count += 1
                                if count >= limit:
                                    break

                        if len(result["matches"]) >= 100:
                            break
                    except Exception as exc:
                        result["errors"].append({"url": clean_url(raw_url), "error": str(exc)})

                result.update({
                    "ok": True,
                    "stage": "public_auth_lifecycle_inspected",
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
