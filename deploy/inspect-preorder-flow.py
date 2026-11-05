#!/usr/bin/env python3
"""Read-only static inspection of Rabin public pre-order UI implementation."""
import json
import re
from urllib.parse import urlsplit, urlunsplit

TARGET = "https://trader.rabin.ir/"
MAX_BUNDLES = 80
MAX_CONTEXTS = 28
MAX_CONTEXT_CHARS = 5600


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
        "note": (
            "Static inspection of public JavaScript only. No login, credentials, "
            "authentication requests, broker API calls, or order actions."
        ),
        "bundles": [],
        "contexts": [],
        "linked_callback_names": [],
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
                    found_positions = set()

                    def add_context(label, pos, before=2200, after=3400):
                        if len(result["contexts"]) >= MAX_CONTEXTS or pos in found_positions:
                            return
                        found_positions.add(pos)
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
                        (r"preOrder\s*:\s*\{[^{}]{0,1800}sendGroupPreOrder", "preorder_route_table"),
                        (r"\b[A-Za-z_$][\w$]*\.Hv\.preOrder\.preOrder", "preorder_create_route_use"),
                        (r"\b[A-Za-z_$][\w$]*\.Hv\.preOrder\.sendGroupPreOrder", "preorder_send_route_use"),
                        (r"\b[A-Za-z_$][\w$]*\.Hv\.preOrder\.sendAllPreOrder", "preorder_send_all_route_use"),
                    )
                    for pattern, label in route_patterns:
                        for index, match in enumerate(re.finditer(pattern, source)):
                            if index >= 3:
                                break
                            add_context(label, match.start(), before=2600, after=4300)

                    # Find the callback passed to the order-entry dialog and inspect
                    # its closest assignment(s), where the POST method and form data
                    # construction should be visible if they are in this public bundle.
                    for match in re.finditer(r"\bpreOrder\s*:\s*([A-Za-z_$][\w$]*)", source):
                        name = match.group(1)
                        result["linked_callback_names"].append(name)
                        assignment = re.compile(r"\b" + re.escape(name) + r"\s*=")
                        occurrences = list(assignment.finditer(source))
                        occurrences.sort(key=lambda item: abs(item.start() - match.start()))
                        for item in occurrences[:6]:
                            add_context(
                                "preOrder_prop_callback_" + name,
                                item.start(),
                                before=3200,
                                after=5200,
                            )
                        if len(result["contexts"]) >= MAX_CONTEXTS:
                            break

                    if result["contexts"]:
                        result["contexts"] = result["contexts"][:MAX_CONTEXTS]
                    if len(result["contexts"]) >= MAX_CONTEXTS:
                        break
                except Exception:
                    continue

            result.update({
                "ok": True,
                "stage": "public_preorder_flow_inspected",
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
