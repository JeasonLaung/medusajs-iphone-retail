"""Export schema/count evidence only. Never serialize source row values to docs."""
import argparse
import csv
import hashlib
import json
import subprocess
from collections import Counter
from decimal import Decimal
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "docs/migration"


def classify(name):
    if name in {"auth_identity", "auth_mfa_factor", "auth_mfa_recovery_code", "auth_password_reset_token", "auth_verification", "provider_identity", "user", "invite", "invite_rbac_role", "user_rbac_role", "user_preference", "api_key", "publishable_api_key_sales_channel"}:
        return "exclude-login", "identity", "T014,T015,T030", "不导入旧登录身份/凭据/令牌/个人授权；新系统重新创建"
    if name in {"customer", "customer_group", "customer_group_customer"}:
        return "history-subject-only", "historical_subject", "T006,T027,T034", "仅设计历史主体/分组快照映射，不生成新客户账号或按邮箱归属"
    if name in {"customer_address", "customer_account_holder"}:
        return "exclude-account-profile", "identity", "T006,T030", "不继承旧账号地址簿/绑定；订单地址及支付事实另行保留"
    if name.startswith("rbac_"):
        return "rebuild-config", "permissions", "T004,T015", "按新动作和范围重建权限模型，不导入旧账号授权关系"
    if name in {"locking", "workflow_execution", "mikro_orm_migrations", "link_module_migrations", "script_migrations"}:
        return "rebuild-runtime", "platform", "T011,T017,T018", "不重放旧运行实例/锁/迁移账本；未完成外部操作单独对账"
    if name.startswith("search_"):
        return "rebuild-derived", "search", "T022", "由已迁移有效商品重建索引，不照搬 PostgreSQL 搜索产物"
    if name in {"layout_configuration", "view_configuration", "property_label"}:
        return "review-ui-config", "admin", "T023,T024,T038", "核对实际定制，迁入新 UI 配置或记录不适用原因"
    if name in {"mail_account", "mollie_config", "paypal_connection", "paypal_settings", "seo_provider", "webhook_endpoint"}:
        return "protected-config-review", "integrations", "T016,T035,T055,T057,T058", "核对加密封装与配置；原始秘密不入 Git，演练不得触发外部副作用"
    if name.startswith("device"):
        return "migrate-business", "device", "T040,T041,T042,T043,T044", "保留身份、来源、软删除及订单/售后/替换关系"
    if name.startswith("buyback"):
        return "migrate-business", "buyback", "T050,T051,T052", "保留申请项、状态、历史与私有文件映射；旧客户仅历史引用"
    if name == "after_sales_request":
        return "migrate-business", "aftersales", "T045,T046,T047", "保留申请、设备、订单行、退款关联和状态"
    if name.startswith("mail_"):
        return "migrate-business", "messaging", "T055,T056", "保留退订、投递和去重；旧验证邮件不重放，队列须分状态对账"
    if name.startswith("seo_"):
        return "migrate-business", "seo", "T058,T059", "保留元信息/事件/监测证据；任务执行状态单独处理"
    if name == "product_review":
        return "migrate-business", "review", "T054", "保留审核、评分、原订单/客户历史关联"
    if name in {"customer_notice", "notification", "notification_provider"}:
        return "review-notification", "messaging", "T056,T057", "区分客户/员工/系统通知，不为旧身份恢复登录；保留必要历史和去重"
    if name == "order_number_clock" or name == "sku_number_counter":
        return "migrate-sequence", "numbering", "T013", "原编号与下一分配位置保留；显式转换触发器和并发规则"
    if name.startswith("order") or name.startswith("return") or name in {"credit_line", "refund", "refund_reason", "capture", "payment", "payment_collection", "payment_session", "paypal_webhook_event", "account_holder"}:
        return "migrate-business", "commerce", "T034,T035,T036,T037,T045,T046,T047", "保留完整历史交易关联、快照和幂等；认证/付款令牌字段须受保护审查"
    if name == "paypal_metric":
        return "review-provider-history", "payment", "T035,T036", "核对插件用途，不能把 provider 统计当交易事实或重复上报"
    if name.startswith("cart"):
        return "review-open-transaction", "checkout", "T032,T033,T036", "区分普通未完成购物车、支付在途与已完成关联；旧登录会话不继承"
    if name.startswith("inventory") or name == "reservation_item":
        return "migrate-business", "inventory", "T031,T037,T044", "保留数量和预留关联，不用设备数覆盖库存"
    if name.startswith("fulfillment") or name.startswith("location_fulfillment") or name.startswith("shipping_") or name in {"service_zone", "geo_zone"}:
        return "migrate-business", "fulfillment", "T033,T037", "区分配置与历史履约、标签；保留数量、来源和区域范围"
    if name.startswith("product") or name == "image":
        return "migrate-business", "catalog", "T020,T024,T026", "保留全部关联、选项归属、图片顺序、软删除与原 ID"
    if name.startswith("price") or name == "currency":
        return "migrate-business", "pricing", "T013,T021", "按币种/条件/数量精确映射，金额不经浮点或统一缩放"
    if name.startswith("promotion") or name.startswith("application_method"):
        return "migrate-business", "promotions", "T033", "保留规则、预算使用、历史订单折扣及条件关系"
    if name.startswith("tax_"):
        return "migrate-business", "tax", "T033", "保留区域/税率/规则；历史成交税快照不重新计算覆盖"
    if name.startswith("stock_location") or name.startswith("sales_channel") or name.startswith("region") or name.startswith("store") or name in {"payment_provider", "payment_collection_payment_providers"}:
        return "migrate-business", "configuration", "T020,T021,T031,T033,T035,T058", "核对有效配置/渠道/地区关联，店铺语言/币种/metadata 不遗漏"
    return "unclassified-review", "unknown", "T006", "必须人工明确去向；不允许默认忽略"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", required=True)
    parser.add_argument("--source-container", default="medusa-trial-postgres-1")
    args = parser.parse_args()
    bundle_path = Path(args.bundle).resolve()
    ignored = subprocess.run(["git", "check-ignore", "--quiet", str(bundle_path)], cwd=ROOT)
    if ignored.returncode != 0:
        raise SystemExit("Source bundle must be Git-ignored")
    raw = bundle_path.read_bytes()
    bundle = json.loads(raw, parse_float=Decimal)
    if bundle.get("format_version") != 1:
        raise SystemExit("Unsupported bundle format")
    sql = """BEGIN READ ONLY;
SELECT coalesce(json_agg(c ORDER BY table_name,ordinal_position),'[]'::json) FROM
(SELECT table_name,column_name,ordinal_position,data_type,udt_name,is_nullable,
character_maximum_length,numeric_precision,numeric_scale,datetime_precision
FROM information_schema.columns WHERE table_schema='public') c;
COMMIT;"""
    result = subprocess.run(["docker", "exec", "-i", args.source_container, "psql", "-X", "-q", "-t", "-A", "-v", "ON_ERROR_STOP=1", "-U", "medusa", "-d", "medusa_trial"], input=sql, capture_output=True, text=True, check=True)
    columns = json.loads(result.stdout.strip())
    tables = []
    for name, rows in sorted(bundle["tables"].items()):
        disposition, domain, tasks, note = classify(name)
        cols = [c for c in columns if c["table_name"] == name]
        if not cols:
            raise SystemExit(f"Missing schema: {name}")
        numeric_profiles = []
        for column in cols:
            if column["data_type"] not in {"numeric", "bigint", "integer", "smallint", "real", "double precision"}:
                continue
            values = [Decimal(str(row[column["column_name"]])) for row in rows if row.get(column["column_name"]) is not None]
            numeric_profiles.append({"column": column["column_name"], "non_null_rows": len(values),
                                     "max_integer_digits": max((max(1, v.adjusted() + 1) for v in values if v.is_finite()), default=0),
                                     "max_fraction_digits": max((max(0, -v.as_tuple().exponent) for v in values if v.is_finite()), default=0),
                                     "non_finite_rows": sum(not v.is_finite() for v in values)})
        tables.append({"source_table": name, "snapshot_rows": len(rows),
                       "snapshot_soft_deleted_rows": sum(r.get("deleted_at") is not None for r in rows),
                       "disposition": disposition, "domain": domain, "tasks": tasks,
                       "status": "mapping-pending", "note": note, "columns": cols,
                       "numeric_profiles": numeric_profiles})
    report = {"format_version": 1, "source_commit": bundle["source_commit"],
              "bundle_sha256": hashlib.sha256(raw).hexdigest(),
              "scope": "2026-10-26 snapshot row counts; current source schema metadata only; no row values",
              "tables": tables}
    OUT.mkdir(parents=True, exist_ok=True)
    (OUT / "data-source-manifest.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    with (OUT / "data-table-disposition.csv").open("w", encoding="utf-8-sig", newline="") as output:
        fields = ["source_table", "snapshot_rows", "snapshot_soft_deleted_rows", "disposition", "domain", "tasks", "status", "note"]
        writer = csv.DictWriter(output, fieldnames=fields, extrasaction="ignore")
        writer.writeheader()
        writer.writerows(tables)
    print(json.dumps({"tables": len(tables), "columns": sum(len(t["columns"]) for t in tables), "snapshot_rows": sum(t["snapshot_rows"] for t in tables), "dispositions": Counter(t["disposition"] for t in tables)}, ensure_ascii=False))


if __name__ == "__main__":
    main()
