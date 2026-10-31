"""Verify the same-source customer supplemental-refund chain and owned cleanup."""
import hashlib
import json
from pathlib import Path
import re
import runpy

ROOT=Path(__file__).resolve().parents[2]

def sha(p):
 return hashlib.sha256(p.read_bytes().replace(b'\r\n',b'\n')).hexdigest()

def main():
 runner=runpy.run_path(str(ROOT/'retail/scripts/test-supplement-refund-local.py'))
 c=runner['config']();snap=Path(c['snapshot'])
 result=json.loads((snap/'joint-result.json').read_text())
 clean=json.loads((snap/'process-cleanup.json').read_text())
 receipt=json.loads((snap/'root-cleanup-receipt.json').read_text())
 if clean.get('owned_processes_terminal') is not True or clean.get('owned_database_absent') is not True or receipt!={'owner':c['owner'],'exit_code':0,'schema_count':0,'user_count':0}:
  raise RuntimeError('Complete owned process/database/Redis and root user cleanup required')
 sql=result['sql'];browser=result['browser']
 flags=['same_four_registered_shopped_customers','native_exchange_supplement_and_replacement_shipment','actual_second_return_request_and_scanned_replacement_receipt','actual_photo_inspection','second_return_completion_rejected_by_source_guard','no_second_return_completed_notice','actual_admin_selects_supplemental_payment','original_cash_not_refunded','separate_production_refund_cli','lost_post_restart_get_only','late_notice_fault_cash_rollback','saved_observation_local_reapply','seven_own_read_notices_per_order','one_refund_per_request_preserves_source']
 if sql.get('completion_conflict_http_status')!=409 or sql.get('completion_conflict_error_code')!='after_sales.unavailable' or any(sql.get(k) is not True for k in flags) or sql.get('provider_refund_posts')!=4 or sql.get('exact_refunded_per_order')!='6.00' or sql.get('final_paid_total')!='164.75' or sql.get('final_refunded_total')!='6' or sql.get('final_pending_difference')!='-150' or sql.get('return_amount_change_applied') is not True or sql.get('real_provider') is not False or sql.get('real_email') is not False:
  raise RuntimeError('Customer supplemental refund facts incomplete')
 if browser.get('same_new_order_supplemental_cash_return_refund_completion_denied_by_source_rule') is not True or browser.get('unique_orders')!=4 or browser.get('browser_errors')!=0 or result.get('source_frozen_before_build_and_verified_after') is not True:
  raise RuntimeError('Actual frozen browser/SQL joint required')
 binaries=['payment-worker','refund-worker']
 if {n:sha(snap/n) for n in binaries}!=result['compiled_cli_sha256']:raise RuntimeError('Executed CLI binary changed')
 binary_sha256={n:hashlib.sha256((snap/n).read_bytes()).hexdigest() for n in binaries}
 sources=c['source_sha256_lf'];adapted=c['adapted_source_sha256_lf']
 if not sources or any(not n.endswith(('_test.go','.mjs')) for n in adapted):raise RuntimeError('Only explicit test/port/namespace adapters allowed')
 for n,d in sources.items():
  if sha(snap/n)!=adapted.get(n,d):raise RuntimeError('Snapshot source changed: '+n)
 for n in runner['OWNED']:
  if sha(ROOT/n)!=sources[n]:raise RuntimeError('Owned worktree source changed: '+n)
 migration_count=sum(n.startswith('retail/backend/migrations/') and n.endswith('.sql') for n in sources)
 if migration_count!=88:raise RuntimeError('Committed 88 migration baseline required')
 text=(snap/'logs/gin-migration/joint-sql.log').read_text()
 if not re.search(r'^--- PASS: TestOwnedCartRuntime \([\d.]+s\)$',text,re.M) or '\nPASS\n' not in text or re.search(r'^--- FAIL:|^FAIL$',text,re.M):raise RuntimeError('Completed native SQL runner required')
 validation=ROOT/'logs/gin-migration/supplement-refund-validation-88'
 terminal=json.loads((validation/'terminal.json').read_text())
 full_sql=(validation/'full-sql.log').read_text()
 suites=[json.loads(line)['running_suite'] for line in full_sql.splitlines() if line.startswith('{"running_suite":')]
 if terminal!={'full_sql_exit_code':0,'go_all_exit_code':0,'go_vet_exit_code':0,'owned_sql_containers_absent':True} or len(suites)!=23 or len(set(suites))!=23 or json.loads(full_sql.splitlines()[-1]).get('cleanup')!='complete' or json.loads(full_sql.splitlines()[-1]).get('counts',{}).get('explicit_migrations')!=88:
  raise RuntimeError('Complete 23 suite SQL, Go checks and fixture cleanup required')
 frozen=json.loads((validation/'owned-source-sha256.json').read_text())
 if frozen!={n:sha(ROOT/n) for n in runner['OWNED']}:raise RuntimeError('Full SQL/Go source differs from joint')
 validation_hashes={p.name:sha(p) for p in validation.iterdir() if p.is_file()}
 script='retail/scripts/report-supplement-refund-joint.py';sources=dict(sources);sources[script]=sha(ROOT/script)
 guard='medusa-store/apps/backend/src/workflows/update-after-sales.ts'
 if 'a.device_id === id && !a.replacement_of' not in (ROOT/guard).read_text():raise RuntimeError('Source replacement completion guard changed')
 sources[guard]=sha(ROOT/guard)
 report=dict(format_version=1,title='补款退款及替换品退货完成限制',full_23_sql_suites_passed=True,unadapted_go_all_and_vet_passed=True,validation_transcript_sha256_lf=validation_hashes,scope='Four actual new registered customers: original checkout, positive exchange, supplemental payment, replacement shipment, actual return and inspected receipt, employee-approved 6 EUR refund to supplemental capture, current owner notice; source replacement_of guard retains second return processing and rejects completion (409). Preserves source one-refund-per-return rule; does not claim full-order refund or zero difference.',baseline_commit=c['commit'],migration_count=migration_count,actual_customer_supplemental_refund_joint=True,return_amount_change_applied=True,second_return_completed=False,full_storefront_accepted=False,real_provider_or_email=False,browser=browser,sql=sql,source_sha256_lf=sources,test_environment_adapted_sha256_lf=adapted,production_source_changed_for_environment=False,compiled_cli_sha256=binary_sha256,source_frozen_before_build_and_verified_after=True,cleanup_complete=True,cleanup=dict(clean,root_user_cleanup_pending=False),root_user_cleanup=receipt,transcript_sha256_lf={str(p.relative_to(snap)):sha(p) for p in [snap/'joint-result.json',snap/'process-cleanup.json',snap/'root-cleanup-receipt.json',snap/'logs/gin-migration/joint-sql.log',snap/'logs/gin-migration/joint-browser.log',snap/'logs/gin-migration/astro-build.log']})
 path=ROOT/'docs/migration/shopping-supplement-refund-joint-report.json';path.write_text(json.dumps(report,indent=2)+'\n')
 print(json.dumps({'supplemental_customer_refund_joint':'passed','source_hashes':len(sources),'full_storefront_accepted':False}))

if __name__=='__main__':main()
