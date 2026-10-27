"""Capture PostgreSQL constraint/index metadata read-only; never read business rows."""
import argparse
import json
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "docs/migration/source-schema-constraints.json"
SQL = """BEGIN READ ONLY;
SELECT json_build_object(
  'constraints', (SELECT coalesce(json_agg(x ORDER BY source_table,constraint_name),'[]'::json) FROM (
    SELECT t.relname AS source_table,c.conname AS constraint_name,c.contype AS constraint_type,
      pg_get_constraintdef(c.oid,true) AS definition,
      CASE WHEN c.confrelid <> 0 THEN r.relname ELSE NULL END AS referenced_table
    FROM pg_constraint c JOIN pg_class t ON t.oid=c.conrelid
    JOIN pg_namespace n ON n.oid=t.relnamespace
    LEFT JOIN pg_class r ON r.oid=c.confrelid
    WHERE n.nspname='public'
  ) x),
  'indexes', (SELECT coalesce(json_agg(x ORDER BY source_table,index_name),'[]'::json) FROM (
    SELECT tablename AS source_table,indexname AS index_name,indexdef AS definition
    FROM pg_indexes WHERE schemaname='public'
  ) x),
  'enums', (SELECT coalesce(json_agg(x ORDER BY type_name,sort_order),'[]'::json) FROM (
    SELECT t.typname AS type_name,e.enumsortorder AS sort_order,e.enumlabel AS label
    FROM pg_type t JOIN pg_enum e ON e.enumtypid=t.oid
    JOIN pg_namespace n ON n.oid=t.typnamespace WHERE n.nspname='public'
  ) x)
);
COMMIT;"""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-container", default="medusa-trial-postgres-1")
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    result = subprocess.run([
        "docker", "exec", "-i", args.source_container, "psql", "-X", "-q", "-t", "-A",
        "-v", "ON_ERROR_STOP=1", "-U", "medusa", "-d", "medusa_trial",
    ], input=SQL, capture_output=True, text=True, encoding="utf-8", check=True)
    captured = json.loads(result.stdout)
    source = json.loads((ROOT / "docs/migration/data-source-manifest.json").read_text(encoding="utf-8-sig"))
    names = {t["source_table"] for t in source["tables"]}
    for kind in ("constraints", "indexes"):
        unexpected = {row["source_table"] for row in captured[kind]} - names
        if unexpected:
            raise ValueError("Source schema has tables outside the recorded manifest; refresh schema inventory first")
    output = {"format_version": 1, "source_commit": source["source_commit"],
              "scope": "Current public constraint/index/enum metadata only; not source rows or target MySQL DDL",
              **captured}
    text = json.dumps(output, ensure_ascii=False, indent=2) + "\n"
    if args.check:
        if OUT.read_text(encoding="utf-8-sig") != text:
            raise ValueError("Source constraint/index/enum evidence changed; review before refreshing")
    else:
        OUT.write_text(text, encoding="utf-8")
    print(json.dumps({"constraints": len(captured["constraints"]), "indexes": len(captured["indexes"]),
                      "enum_values": len(captured["enums"]), "mode": "check" if args.check else "capture"}))


if __name__ == "__main__":
    main()
