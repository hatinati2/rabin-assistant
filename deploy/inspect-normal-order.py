#!/usr/bin/env python3
"""Read-only inspection of the public Rabin UI's NORMAL order submission flow.

This script downloads only public same-origin JavaScript bundles. It does not
log in, inspect credentials, call broker API routes, or place/modify/cancel orders.
It intentionally targets the regular /order flow, not broker pre-orders.
"""
import json
import re
from urllib.parse import urlsplit, urlunsplit

TARGET = "https://trader.rabin.ir/"
MAX_BUNDLES = 80
MAX_CONTEXTS = 26
MAX_CONTEXT_CHARS = 6200


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
        "stage": "starting",
        "purpose": "Inspect regular order placement through /order, not /pre-order.",
        "note": (
            "Read-only static inspection of public JavaScript only. No login, "
            "credentials, authentication calls, broker API requests, or order actions."
        ),
        "bundles": [],
        "contexts": [],
        "order_callback_names": [],
    }

    with sync_playwright() as p:
        browser = None
        try:
            browser = p.chromium.launch(headless=True)
            context = browser.new_context(locale="fa-IR", timezone_id="Asia/Tehran")
            page = context.new_page()
            response = page.goto(TARGET, wait_until="domcontentloaded", timeout=60000)
            page.wait_for_timeout(5000)

            urls = page.evaluate("""() => [...new Set([
              ...[...document.scripts].map(s => s.src).filter(Boolean),
              ...performance.getEntriesByType('resource')
                .filter(e => /\.m?js(?:[?#]|$)/i.test(e.name))
                .map(e => e.name)
            ])].map(raw => {
              try { const u = new URL(raw, location.href); return u.origin === location.origin ? u.origin + u.pathname : ''; }
              catch (_) { return ''; }
            }).filter(Boolean).slice(0, 80)""")

            for raw_url in urls[:MAX_BUNDLES]:
                try:
                    r = context.request.get(raw_url, timeout=25000)
                    if not r.ok:
                        continue
                    source = r.text()
                    if len(source) > 12_000_000:
                        continue

                    bundle = clean_url(raw_url)
                    result["bundles"].append({"url": bundle, "bytes": len(source)})
                    seen = set()

                    def add_context(label, pos, before=2600, after=4300):
                        if len(result["contexts"]) >= MAX_CONTEXTS or (bundle, pos) in seen:
                            return
                        seen.add((bundle, pos))
                        left = max(0, pos - before)
                        right = min(len(source), pos + after)
                        snippet = re.sub(r"\s+", " ", source[left:right])
                        result["contexts"].append({
                            "bundle": bundle,
                            "label": label,
                            "offset": pos,
                            "snippet": snippet[:MAX_CONTEXT_CHARS],
                        })

                    route_patterns = (
                        (r"order\s*:\s*\{[^{}]{0,1800}getAllOrder", "normal_order_route_table"),
                        (r"\b[A-Za-z_$][\w$]*\.Hv\.order\.order\b", "normal_order_api_use"),
                        (r"submitOrder\s*:", "submit_order_state_reference"),
                    )
                    for pattern, label in route_patterns:
                        for index, match in enumerate(re.finditer(pattern, source)):
                            if index >= 3:
                                break
                            add_context(label, match.start(), before=2300, after=4300)

                    # In the order-entry dialog the normal-order callback is passed
                    # as order:<local identifier>, preOrder:<another identifier>.
                    # Follow the local identifier back to its function/API hook.
                    prop_pattern = r"\border\s*:\s*([A-Za-z_$][\w$]*)\s*,\s*preOrder\s*:"
                    for match in re.finditer(prop_pattern, source):
                        name = match.group(1)
                        if name not in result["order_callback_names"]:
                            result["order_callback_names"].append(name)
                        assignment = re.compile(r"\b" + re.escape(name) + r"\s*=")
                        candidates = list(assignment.finditer(source))
                        candidates.sort(key=lambda item: abs(item.start() - match.start()))
                        for item in candidates[:5]:
                            add_context(
                                "normal_order_callback_" + name,
                                item.start(),
                                before=3400,
                                after=5200,
                            )
                        if len(result["contexts"]) >= MAX_CONTEXTS:
                            break

                    if len(result["contexts"]) >= MAX_CONTEXTS:
                        break
                except Exception:
                    continue

            result.update({
                "ok": True,
                "stage": "public_normal_order_flow_inspected",
                "http_status": response.status if response else None,
                "bundle_count": len(result["bundles"]),
                "context_count": len(result["contexts"]),
            })
            print(json.dumps(result, ensure_ascii=False, indent=2), flush=True)
            context.close()
            return 0
        except Exception as exc:
            result.update({"stage": "browser_error", "error": str(exc)})
            print(json.dumps(result, ensure_ascii=False, indent=2), flush=True)
            return 1
        finally:
            if browser is not None:
                browser.close()


if __name__ == "__main__":
    raise SystemExit(main())
