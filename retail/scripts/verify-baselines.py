"""Check source evidence and optional ignored screenshot reports without exposing rows."""
import argparse
import hashlib
import json
import re
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def read_json(path):
    return json.loads(path.read_text(encoding="utf-8-sig"))


def local_path(value, parent=ROOT):
    path = (parent / value).resolve()
    if not path.is_relative_to(parent.resolve()):
        raise ValueError("Evidence path escapes its declared root")
    return path


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def require(condition, message):
    if not condition:
        raise ValueError(message)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--captures", nargs="*", default=[], help="Git-ignored capture directories relative to the repository")
    args = parser.parse_args()
    manifest = read_json(ROOT / "docs/migration/source-manifest.json")
    require(manifest["format_version"] == 2, "Unsupported source manifest")
    for name, expected in manifest["source_sha256"].items():
        require(digest(local_path(name)) == expected, f"Source changed since evidence capture: {name}")
    for route in manifest["routes"]:
        lines = local_path(route["source"]).read_text(encoding="utf-8-sig").splitlines()
        for method in route["methods"]:
            line = method["line"]
            require(0 < line <= len(lines), "Route evidence line is outside source")
            expression = r"export\s+(?:(?:async\s+)?function|const)\s+" + re.escape(method["method"]) + r"\b"
            require(re.search(expression, lines[line - 1]), "Route method evidence no longer matches")
    pages = manifest["storefront_page_patterns"]
    expected_pages = set()
    for path in (ROOT / "medusa-store/apps/storefront/src/app").rglob("page.tsx"):
        expected_pages.add(path.relative_to(ROOT).as_posix())
    recorded_pages = [entry["source"] for entries in pages.values() for entry in entries]
    require(len(recorded_pages) == len(set(recorded_pages)), "Duplicate page evidence")
    require(set(recorded_pages) == expected_pages, "Page evidence does not cover current page files")
    for url, entries in pages.items():
        for entry in entries:
            segments = local_path(entry["source"]).parent.relative_to(ROOT / "medusa-store/apps/storefront/src/app").parts
            derived = "/" + "/".join(s for s in segments if not s.startswith(("(", "@")))
            require(derived == url, "Page URL pattern disagrees with source")
    data = read_json(ROOT / "docs/migration/data-source-manifest.json")
    tables = data["tables"]
    require(len({table["source_table"] for table in tables}) == len(tables), "Duplicate source table")
    for table in tables:
        require(table["columns"], "Missing schema evidence")
        require(table["disposition"] != "unclassified-review", "Unclassified table remains")
        require(all(c["table_name"] == table["source_table"] for c in table["columns"]), "Column/table evidence mismatch")
        require(0 <= table["snapshot_soft_deleted_rows"] <= table["snapshot_rows"], "Invalid aggregate row count")
    captures, errors, bad_status, overflow, broken = 0, 0, 0, 0, 0
    for directory in args.captures:
        folder = local_path(directory)
        report_path = folder / "capture.json"
        ignored = subprocess.run(["git", "check-ignore", "--quiet", str(report_path)], cwd=ROOT, capture_output=True)
        require(ignored.returncode == 0, "Capture reports must remain Git-ignored")
        report = read_json(report_path)
        for record in report["records"]:
            require(digest(local_path(record["file"], folder)) == record["sha256"], "Screenshot checksum mismatch")
            captures += 1
            errors += bool(record["errors"])
            bad_status += record["status"] != 200
            overflow += record["document"]["scroll_width"] > record["width"]
            broken += record["document"]["broken_images"]
    print(json.dumps({
        "scope": "evidence integrity only; not migration or visual acceptance",
        "source_hashes_verified": len(manifest["source_sha256"]),
        "route_methods_verified": sum(len(r["methods"]) for r in manifest["routes"]),
        "page_files": len(recorded_pages), "page_patterns": len(pages),
        "frontend_handlers": len(manifest["storefront_route_handlers"]),
        "source_tables": len(tables), "source_columns": sum(len(t["columns"]) for t in tables),
        "snapshot_rows": sum(t["snapshot_rows"] for t in tables),
        "screenshots_verified": captures, "page_error_instances": errors,
        "non_200_captures": bad_status, "horizontal_overflow_captures": overflow, "broken_images": broken,
    }))


if __name__ == "__main__":
    main()
