"""Read source only; regenerate the migration evidence manifest without loading apps/env."""
import hashlib
import json
import re
from pathlib import Path
from urllib.parse import quote

ROOT = Path(__file__).resolve().parents[2]
BACKEND = ROOT / "medusa-store/apps/backend"
STORE = ROOT / "medusa-store/apps/storefront/src"
OUT = ROOT / "docs/migration"


def source(path):
    return path.read_text(encoding="utf-8-sig")


def relative(path):
    return path.relative_to(ROOT).as_posix()


def target(route):
    if "/shipping-demo" in route:
        return ["T059"]
    if "buyback" in route:
        return ["T050", "T051", "T052", "T053"]
    if "after-sales" in route:
        return ["T045", "T046", "T047", "T049"]
    if "product-reviews" in route:
        return ["T054"]
    if "warehouse-access" in route:
        return ["T015", "T042"]
    if "/devices/refunds" in route:
        return ["T047"]
    if "/devices/supplier-accounts" in route:
        return ["T014", "T015", "T042"]
    if "mobile-intake" in route or "/devices" in route:
        return ["T040", "T041", "T042", "T043", "T048"]
    if "/inventory-items" in route or "/stock-locations" in route:
        return ["T031", "T042"]
    if "/orders" in route:
        return ["T034", "T037", "T038", "T039", "T044"]
    if "webhook-notifications" in route:
        return ["T057"]
    if "/mail" in route:
        return ["T055", "T056"]
    if "notifications" in route:
        return ["T057"]
    if "mollie" in route:
        return ["T035", "T036"]
    if "/seo" in route:
        return ["T058", "T059"]
    if "customer-service" in route or "buy-policies" in route:
        return ["T058", "T059"]
    if "/products" in route or "/variants" in route:
        return ["T020", "T021", "T022"]
    return ["T002"]


def link(path, line=None):
    suffix = f"#L{line}" if line else ""
    return f"[{path}{':' + str(line) if line else ''}](../../{quote(path, safe='/.-_')}{suffix})"


def main():
    routes = []
    for path in sorted((BACKEND / "src/api").rglob("route.ts")):
        text = source(path)
        route = "/" + path.parent.relative_to(BACKEND / "src/api").as_posix()
        methods = []
        for match in re.finditer(r"export\s+(?:(?:async\s+)?function|const)\s+(GET|POST|DELETE|PATCH|PUT|OPTIONS|HEAD)\b", text):
            methods.append({"method": match[1], "line": text.count("\n", 0, match.start()) + 1})
        if not methods:
            raise ValueError(f"Unrecognized route export: {path}")
        imports = re.findall(r'from\s+["\']([^"\']+)["\']', text)
        routes.append({"path": route, "source": relative(path), "methods": methods,
                       "workflow_imports": [i for i in imports if "workflows/" in i or "core-flows" in i],
                       "tasks": target(route), "authorization": "pending-T004", "status": "source-evidence-only"})

    sdk_calls = []
    # A lexical scan deliberately records candidates, not confirmed live traffic.
    for path in sorted(STORE.rglob("*")):
        if path.suffix not in {".ts", ".tsx"}:
            continue
        text = source(path)
        for match in re.finditer(r"\bsdk\s*\.\s*(?:store|auth)\s*\.\s*\w+(?:\s*\.\s*\w+)*\s*\(", text):
            expression = re.sub(r"\s+", "", match[0])[:-1]
            sdk_calls.append({"expression": expression, "source": relative(path),
                              "line": text.count("\n", 0, match.start()) + 1})

    groups = []
    for path in sorted((BACKEND / "src/api").rglob("*validators.ts")):
        text = source(path)
        actions = sorted(set(re.findall(r'action\s*:\s*z\.literal\(["\']([^"\']+)["\']\)', text)))
        for match in re.finditer(r"action\s*:\s*z\.enum\(\[([^]]+)\]", text):
            actions += re.findall(r'["\']([^"\']+)["\']', match[1])
        if actions:
            groups.append({"source": relative(path), "actions": sorted(set(actions))})

    modules = []
    for path in sorted((BACKEND / "src/modules").glob("*/index.ts")):
        base = path.parent
        models = []
        for model in sorted((base / "models").glob("*.ts")):
            models += re.findall(r'\.define\(\s*["\']([^"\']+)["\']', source(model))
        modules.append({"name": base.name, "source": relative(path), "models": models})
    assets = {}
    for name, folder, pattern in [
        ("admin_pages", "src/admin/routes", "page.tsx"),
        ("widgets", "src/admin/widgets", "*.tsx"),
        ("workflows", "src/workflows", "*.ts"),
        ("jobs", "src/jobs", "*.ts"),
        ("subscribers", "src/subscribers", "*.ts"),
    ]:
        base = BACKEND / folder
        paths = base.rglob(pattern) if name == "admin_pages" else base.glob(pattern)
        assets[name] = [relative(p) for p in sorted(paths) if not p.name.endswith((".test.ts", ".spec.ts"))]

    pages = {}
    for path in sorted((STORE / "app").rglob("page.tsx")):
        segments = path.parent.relative_to(STORE / "app").parts
        url = "/" + "/".join(s for s in segments if not s.startswith(("(", "@")))
        state = "dashboard" if "@dashboard" in segments else "login" if "@login" in segments else "page"
        pages.setdefault(url, []).append({"source": relative(path), "state": state})
    frontend_handlers = []
    for path in sorted((STORE / "app").rglob("route.ts")):
        text = source(path)
        methods = re.findall(r"export\s+(?:(?:async\s+)?function|const)\s+(GET|POST|DELETE|PATCH|PUT|OPTIONS|HEAD)\b", text)
        frontend_handlers.append({"path": "/" + path.parent.relative_to(STORE / "app").as_posix(),
                                  "source": relative(path), "methods": methods})

    tasks = set(re.findall(r"^\| (T\d+) \|", source(ROOT / "docs/gin-react-astro-tasks.md"), re.M))
    for route in routes:
        assert all(t in tasks for t in route["tasks"]), route
    paths = sorted({r["source"] for r in routes} | {c["source"] for c in sdk_calls}
                   | {g["source"] for g in groups} | {m["source"] for m in modules}
                   | {p for entries in assets.values() for p in entries}
                   | {s["source"] for entries in pages.values() for s in entries}
                   | {h["source"] for h in frontend_handlers})
    hashes = {p: hashlib.sha256((ROOT / p).read_bytes()).hexdigest() for p in paths}
    manifest = {"format_version": 2, "scope": "static-source-evidence; not runtime verification",
                "source_sha256": hashes,
                "routes": routes, "storefront_sdk_call_candidates": sdk_calls,
                "command_validators": groups, "modules": modules,
                "storefront_page_patterns": pages, "storefront_route_handlers": frontend_handlers, **assets}
    OUT.mkdir(parents=True, exist_ok=True)
    (OUT / "source-manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    lines = ["# 源码证据追踪表", "", "由 `retail/scripts/inventory-business.py` 生成。只读扫描源码，不加载环境变量或连接数据库。",
             "", "本表记录静态证据及任务去向，**不代表运行验证、权限审计或迁移完成**。动态调用、间接封装、插件和 Medusa 内置 API 需另行核对；未找到调用方不表示功能可删除。",
             "", "## 自定义接口", "", "全局中间件与路由内校验共同决定权限；所有接口的数据范围仍待 T004 审计。任务为领域承接范围，具体 HTTP/动作契约由 T007 定义。",
             "", "| 旧路由 | 方法与证据 | 直接工作流依赖 | 承接任务 |", "| --- | --- | --- | --- |"]
    for r in routes:
        evidence = "、".join(link(r["source"], m["line"]) + " " + m["method"] for m in r["methods"])
        flows = "、".join("`" + i + "`" for i in r["workflow_imports"]) or "需追踪委托/模块调用"
        lines.append(f'| `{r["path"]}` | {evidence} | {flows} | {", ".join(r["tasks"])} |')
    lines += ["", "## 前台原生 SDK 调用候选", "", "包括可能未使用的导出函数；调用是否可达及参数、错误分支需人工追踪。直接 `/store/*` 请求另见迁移盘点，不能只覆盖 SDK 方法。", "", "| SDK 表达式 | 源码 |", "| --- | --- |"]
    for call in sdk_calls:
        lines.append(f'| `{call["expression"]}` | {link(call["source"], call["line"])} |')
    lines += ["", "## 一个接口中的多个命令", "", "这里只列验证器里的 action；operation、问卷分支与工作流内部状态仍要细分，不能以一个 POST 算一个功能。", "", "| 验证器 | action |", "| --- | --- |"]
    for group in groups:
        lines.append(f'| {link(group["source"])} | {", ".join(group["actions"])} |')
    for key, paths in assets.items():
        lines += ["", f"## {key}（{len(paths)}）", ""]
        lines += ["- " + link(p) for p in paths]
    lines += ["", "## 自定义模块", "", "| 模块 | 实体 |", "| --- | --- |"]
    for mod in modules:
        lines.append(f'| {link(mod["source"])} | {", ".join(mod["models"]) or "Provider / 数据库编号迁移；需单独审查"} |')
    (OUT / "source-traceability.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
    page_lines = ["# 前台路由模式与实现状态入口", "",
                  "由 `retail/scripts/inventory-business.py` 生成。路由组与 parallel slot 从 URL 去除，保留各 slot 的源码证据；本表不证明页面已采集或迁移。",
                  "", f"共 {sum(map(len, pages.values()))} 个 page 文件、{len(pages)} 个去重页面模式，以及 {len(frontend_handlers)} 个 route handler。",
                  "", "`[countryCode]` 实际使用国家-语言组合（例如 dk-en / dk-zh），不是仅国家码。账号 layout 根据 retrieveCustomer 选择 dashboard/login slot；某个 URL 缺少 login 实现时不能假设有完整登录前页面。",
                  "", "| URL 模式 | 实现入口 / 状态 |", "| --- | --- |"]
    for url, entries in sorted(pages.items()):
        page_lines.append(f'| `{url}` | ' + "；".join(f'{s["state"]}: {link(s["source"])}' for s in entries) + " |")
    page_lines += ["", "## 非页面入口", "", "GET 不自动等于无副作用；支付/验证/转移令牌入口采集前需审查。SEO 文件与支付兼容 URL 也属于 Astro 迁移范围。", "", "| URL | 方法 | 源码 |", "| --- | --- | --- |"]
    for handler in frontend_handlers:
        page_lines.append(f'| `{handler["path"]}` | {", ".join(handler["methods"])} | {link(handler["source"])} |')
    (OUT / "frontend-routes.md").write_text("\n".join(page_lines) + "\n", encoding="utf-8")
    print(json.dumps({"route_files": len(routes), "method_exports": sum(len(r["methods"]) for r in routes),
                      "sdk_candidates": len(sdk_calls), "modules": len(modules),
                      "storefront_page_patterns": len(pages), "storefront_route_handlers": len(frontend_handlers),
                      **{k: len(v) for k, v in assets.items()}}))


if __name__ == "__main__":
    main()
