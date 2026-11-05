#!/usr/bin/env python3
"""Read-only inspection of Rabin's public frontend order-side enum mapping.

Only downloads public JavaScript bundles from trader.rabin.ir. It does not log in,
read browser storage, send API calls, or submit any order.
"""
import json
import re
from urllib.parse import urlsplit, urlunsplit

TARGET = "https://trader.rabin.ir/"
MAX_BUNDLES = 120
MAX_BYTES = 20_000_000

def clean_url(raw):
    p = urlsplit(raw)
    return urlunsplit((p.scheme, p.netloc, p.path, "", ""))

def main():
    try:
        from playwright.sync_api import sync_playwright
    except Exception as exc:
        print(json.dumps({"ok": False, "stage": "import", "error": str(exc)}, ensure_ascii=False))
        return 2

    result = {
        "ok": False,
        "purpose": "Identify the public frontend enum values used for BUY/SELL in normal POST /order.",
        "safety": "Public JavaScript GETs only; no login, tokens, browser storage, account APIs, or order submissions.",
        "bundles": [],
        "order_side_contexts": [],
        "referenced_module_candidates": [],
        "errors": [],
    }

    try:
        with sync_playwright() as p:
            browser = p.chromium.launch(headless=True)
            try:
                context = browser.new_context(locale="fa-IR", timezone_id="Asia/Tehran")
                page = context.new_page()
                response = page.goto(TARGET, wait_until="domcontentloaded", timeout=60000)
                page.wait_for_timeout(2200)
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
                        js = context.request.get(raw_url, timeout=25000)
                        if not js.ok:
                            continue
                        source = js.text()
                        if len(source) > MAX_BYTES:
                            continue
                        bundle = clean_url(raw_url)
                        result["bundles"].append({"url": bundle, "bytes": len(source)})

                        for match in re.finditer(r"orderSide\\s*:\\s*([A-Za-z_$][\\w$]*)\\.([A-Za-z_$][\\w$]*)", source):
                            alias, prop = match.group(1), match.group(2)
                            left = max(0, match.start() - 1800)
                            right = min(len(source), match.end() + 2200)
                            context_text = re.sub(r"\\s+", " ", source[left:right])
                            result["order_side_contexts"].append({
                                "bundle": bundle,
                                "alias": alias,
                                "property": prop,
                                "offset": match.start(),
                                "context": context_text[:4200],
                            })

                            # Find the imported webpack module associated with the alias.
                            region_start = max(0, match.start() - 12000)
                            prefix = source[region_start:match.start()]
                            imports = list(re.finditer(r"(?:^|[,;])\\s*" + re.escape(alias) + r"\\s*=\\s*(?:[A-Za-z_$][\\w$]*\\.)?\\(?([0-9]{2,8})\\)?", prefix))
                            if imports:
                                module_id = imports[-1].group(1)
                                if module_id not in [x["module_id"] for x in result["referenced_module_candidates"]]:
                                    result["referenced_module_candidates"].append({
                                        "module_id": module_id,
                                        "alias": alias,
                                        "property": prop,
                                        "import_context": re.sub(r"\\s+", " ", prefix[max(0, imports[-1].start()-250):imports[-1].end()+250]),
                                    })

                        # Also retain direct enum-like assignments close to order implementation.
                        if "/order" in source and ("orderSide" in source or "validityType" in source):
                            for pat in (r"(?:exports|[A-Za-z_$][\\w$]*)\\.(?:T|Pn)\\s*=", r"\\b(?:T|Pn)\\s*:\\s*[^,;}]{1,80}"):
                                for m in re.finditer(pat, source):
                                    if len(result["order_side_contexts"]) >= 30:
                                        break
                                    snippet = re.sub(r"\\s+", " ", source[max(0,m.start()-180):min(len(source),m.end()+240)])
                                    if any(x in snippet for x in ("Buy","Sell","خرید","فروش","OrderSide","orderSide")):
                                        result["order_side_contexts"].append({"bundle":bundle,"offset":m.start(),"enum_candidate":snippet})

                        if len(result["order_side_contexts"]) >= 30:
                            break
                    except Exception as exc:
                        result["errors"].append({"url": clean_url(raw_url), "error": str(exc)})

                result.update({
                    "ok": True,
                    "stage": "order_side_frontend_enum_inspected",
                    "http_status": response.status if response else None,
                    "bundle_count": len(result["bundles"]),
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
