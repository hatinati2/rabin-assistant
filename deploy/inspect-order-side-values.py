#!/usr/bin/env python3
"""Read-only extraction of the exact Rabin BUY/SELL enum values.

Downloads only the public trader.rabin.ir JavaScript bundle. It does not log in,
read browser storage, call account APIs, or submit orders.
"""
import json
import re
from urllib.parse import urlsplit, urlunsplit

BUNDLE = "https://trader.rabin.ir/static/js/main.0348951c.js"
ORDER_OFFSET = 249431
MAX_BUNDLE = 20_000_000

def clean(text):
    return re.sub(r"\s+", " ", text)

def module_ranges(source):
    # Webpack's numeric module table: 1234:function(module,exports,require){...}
    pat = re.compile(r"(?:^|[,{])\s*(\d{1,6}):function\s*\(")
    matches = list(pat.finditer(source))
    for i, match in enumerate(matches):
        start = match.start()
        end = matches[i + 1].start() if i + 1 < len(matches) else len(source)
        yield match.group(1), start, end

def main():
    try:
        from playwright.sync_api import sync_playwright
    except Exception as exc:
        print(json.dumps({"ok": False, "stage": "import", "error": str(exc)}, ensure_ascii=False))
        return 2

    result = {
        "ok": False,
        "purpose": "Extract exact public enum values for c.T (BUY) and c.Pn (SELL) used by normal POST /order.",
        "safety": "One public JavaScript GET only; no login, tokens, browser storage, private APIs, or order submission.",
        "bundle": BUNDLE,
        "order_module": None,
        "enum_module_id": None,
        "enum_module_context": None,
        "matching_definitions": [],
        "errors": [],
    }

    try:
        with sync_playwright() as p:
            request = p.request.new_context()
            response = request.get(BUNDLE, timeout=60000)
            if not response.ok:
                result.update({"stage": "bundle_download", "http_status": response.status})
                print(json.dumps(result, ensure_ascii=False, indent=2))
                return 1
            source = response.text()
            if len(source) > MAX_BUNDLE:
                raise RuntimeError("bundle exceeded size limit")

            order_offset = source.find('orderSide:"BUY"')
            if order_offset < 0:
                order_offset = source.find('orderSide:\"BUY\"')
            if order_offset < 0:
                order_offset = source.find('orderSide')
            if order_offset < 0:
                raise RuntimeError("normal orderSide expression not found")

            ranges = list(module_ranges(source))
            order_module = None
            for module_id, start, end in ranges:
                if start <= order_offset < end:
                    order_module = (module_id, start, end)
                    break
            if not order_module:
                raise RuntimeError("could not locate webpack module containing order payload")

            order_id, order_start, order_end = order_module
            order_text = source[order_start:order_end]
            local_offset = order_offset - order_start
            result["order_module"] = {
                "module_id": order_id,
                "order_side_expression": clean(source[max(0, order_offset-220):order_offset+320])[:700],
                "module_prefix": clean(order_text[:min(local_offset, 12000)])[-5000:],
            }

            # Resolve the minified local variable c from its require() assignment.
            import_matches = list(re.finditer(
                r"(?<![\w$])c\s*=\s*(?:n|__webpack_require__)\s*\(\s*([0-9]{1,8})\s*\)",
                order_text[:local_offset]
            ))
            enum_id = import_matches[-1].group(1) if import_matches else None
            result["enum_module_id"] = enum_id

            if enum_id:
                for module_id, start, end in ranges:
                    if module_id == enum_id:
                        enum_text = source[start:end]
                        result["enum_module_context"] = clean(enum_text[:14000])[:12000]
                        break

            # Also locate exact exports or enum assignment strings throughout the bundle.
            patterns = [
                r".{0,250}(?:Pn|T)\s*[:=].{0,250}",
                r".{0,250}(?:BUY|SELL).{0,250}",
                r".{0,250}OrderSide.{0,250}",
            ]
            for pattern in patterns:
                for match in re.finditer(pattern, source, re.I):
                    snippet = clean(match.group(0))
                    if any(token in snippet for token in ("BUY", "SELL", "Pn", "OrderSide")):
                        if snippet not in result["matching_definitions"]:
                            result["matching_definitions"].append(snippet[:700])
                    if len(result["matching_definitions"]) >= 35:
                        break
                if len(result["matching_definitions"]) >= 35:
                    break

            result.update({
                "ok": True,
                "stage": "enum_module_resolved" if enum_id else "order_module_found_enum_import_unresolved",
                "bundle_bytes": len(source),
                "order_module_id": order_id,
            })
            request.dispose()
    except Exception as exc:
        result.update({"ok": False, "stage": "inspection_error", "error": str(exc)})

    print(json.dumps(result, ensure_ascii=False, indent=2), flush=True)
    return 0 if result["ok"] else 1

if __name__ == "__main__":
    raise SystemExit(main())
