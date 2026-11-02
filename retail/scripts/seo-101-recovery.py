"""Narrow inverse of unactivated0101, shared by clone rehearsal and main recovery.

Caller must stop the sole application and obtain fresh full-table fingerprints.
Refuses new facts, grants, enabled gates, unknown schema or unrelated changes.
"""
import hashlib
import re


class RecoveryRefused(RuntimeError):
    pass


def require(ok, message):
    if not ok:
        raise RecoveryRefused(message)


def restore(query, before, actual, authority):
    require(query('SELECT DATABASE(),@@innodb_native_foreign_keys,@@foreign_key_checks,@@skip_networking;') == [['eurorefurb_local_trial','1','1','1']], 'Private native-FK recovery target required')
    allowed = {'authorization_actions','authorization_runtime_schema','schema_migrations','schema_migration_steps'}
    additions = {'seo_command_state','seo_command_receipts'}
    require(set(before) <= set(actual) and set(actual)-set(before) <= additions, 'Recovery refused: unexpected table inventory')
    require(all(actual[t] == f for t,f in before.items() if t not in allowed), 'Recovery refused: original business facts changed')
    actions, runtime = authority['actions'], authority['runtime']
    require(len(actions) == 33 and actions == sorted(actions) and len({r[0] for r in actions}) == 33 and all(len(r) == 1 and re.fullmatch('[a-z][a-z._-]{1,79}',r[0]) for r in actions), 'Invalid original action authority')
    require(len(runtime) == 1 and len(runtime[0]) == 5 and runtime[0][:2] == ['1','1'] and runtime[0][4] in ['0','1'] and runtime[0][3].isdigit() and int(runtime[0][3]) > 0, 'Invalid original runtime authority')
    original_hash = hashlib.sha256(('\n'.join(r[0] for r in actions)+'\n').encode()).hexdigest()
    require(runtime[0][2] == original_hash and not any(r[0] in ['seo.read','seo.write'] for r in actions), 'Original model authority differs')
    current_actions = query('SELECT code FROM authorization_actions ORDER BY code;')
    expanded_actions = sorted(actions + [['seo.read'],['seo.write']])
    require(current_actions in [actions,expanded_actions], 'Recovery refused: authorization vocabulary changed')
    expanded_runtime = [list(runtime[0])]
    expanded_runtime[0][2] = hashlib.sha256(('\n'.join(r[0] for r in expanded_actions)+'\n').encode()).hexdigest()
    expanded_runtime[0][3] = str(int(runtime[0][3])+1)
    require(query('SELECT id,schema_version,model_sha256,revision,enabled FROM authorization_runtime_schema;') in [runtime,expanded_runtime], 'Recovery refused: authorization runtime changed')
    require(query("SELECT COUNT(*) FROM authorization_role_actions WHERE action_code IN ('seo.read','seo.write');") == [['0']], 'Recovery refused: new SEO grants exist')
    checks = query("SELECT constraint_name FROM information_schema.table_constraints WHERE constraint_schema=DATABASE() AND table_name='authorization_actions' AND constraint_type='CHECK' ORDER BY constraint_name;")
    require(checks in [[],[['ck_authorization_action_v15']],[['ck_authorization_action_v16']]], 'Recovery refused: unexpected action constraints')
    if 'seo_command_state' in actual:
        require(query('SELECT id,version,enabled FROM seo_command_state;') in [[],[['1','1','0']]], 'Recovery refused: SEO activated or state changed')
    if 'seo_command_receipts' in actual:
        require(query('SELECT COUNT(*) FROM seo_command_receipts;') == [['0']], 'Recovery refused: SEO receipts exist')
    # All guard reads finish before the first mutation. Foreign keys stay ON.
    if 'seo_command_receipts' in actual:
        query('DROP TABLE seo_command_receipts;')
    if 'seo_command_state' in actual:
        query('DROP TABLE seo_command_state;')
    query("DELETE FROM authorization_actions WHERE code IN ('seo.read','seo.write');")
    if checks == [['ck_authorization_action_v16']]:
        query('ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v16;')
    if checks != [['ck_authorization_action_v15']]:
        codes=','.join("_utf8mb4'"+r[0]+"'" for r in actions)
        query('ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v15 CHECK(code IN ('+codes+'));')
    query("UPDATE authorization_runtime_schema SET model_sha256='"+original_hash+"',revision="+runtime[0][3]+" WHERE id=1;")
    query("DELETE FROM schema_migration_steps WHERE version='0101_seo_admin.sql'; DELETE FROM schema_migrations WHERE version='0101_seo_admin.sql';")
    require(query('SELECT code FROM authorization_actions ORDER BY code;') == actions and query('SELECT id,schema_version,model_sha256,revision,enabled FROM authorization_runtime_schema;') == runtime, 'Original authorization recovery differs')
    return {'inverse_applied':True,'foreign_keys_kept_on':True,'new_grants_absent':True,'new_receipts_absent':True,'activated_state_refused':True}
