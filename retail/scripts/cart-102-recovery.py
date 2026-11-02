"""Narrow inverse of unactivated0102 for private clone/main recovery only.

Caller stops the sole API and supplies fresh whole-table row fingerprints and
the journal authority from its backup. New cart facts/gate activation refuse
recovery; native foreign keys stay enabled. No provider or application runs.
"""
import re


class RecoveryRefused(RuntimeError):
    pass


def require(ok, message):
    if not ok:
        raise RecoveryRefused(message)


def restore(query, before, actual, authority):
    require(query('SELECT DATABASE(),@@innodb_native_foreign_keys,@@foreign_key_checks,@@skip_networking;') == [['eurorefurb_local_trial','1','1','1']], 'Private native-FK recovery target required')
    journals = {'schema_migrations','schema_migration_steps'}
    additions = {'cart_context_runtime','cart_context_operations'}
    require(set(before) <= set(actual) and set(actual)-set(before) <= additions, 'Recovery refused: unexpected table inventory')
    require(all(actual[t] == f for t,f in before.items() if t not in journals), 'Recovery refused: original business facts changed')
    journal = authority['journal']
    checksum = authority['new_checksum']
    require(len(journal)==101 and journal==sorted(journal) and len({r[0] for r in journal})==101 and all(len(r)==2 and re.fullmatch(r'\d{4}_[a-z0-9_]+\.sql',r[0]) and re.fullmatch('[a-f0-9]{64}',r[1]) for r in journal), 'Invalid original migration authority')
    require(re.fullmatch('[a-f0-9]{64}',checksum) is not None and all(r[0]<'0102_cart_context.sql' for r in journal), 'Invalid new migration authority')
    current = query('SELECT version,checksum FROM schema_migrations ORDER BY version;')
    require(current in [journal,journal+[['0102_cart_context.sql',checksum]]], 'Recovery refused: migration vocabulary changed')
    require(query("SELECT COUNT(*) FROM schema_migration_steps WHERE version<>'0102_cart_context.sql' AND status<>'applied';") == [['0']], 'Recovery refused: original migration incomplete')
    if 'cart_context_runtime' in actual:
        require(query('SELECT id,version,enabled FROM cart_context_runtime;') in [[],[['1','1','0']]], 'Recovery refused: cart context activated or state changed')
    if 'cart_context_operations' in actual:
        require(query('SELECT COUNT(*) FROM cart_context_operations;') == [['0']], 'Recovery refused: cart context receipts exist')
    # Finish all guards before the first mutation. Drop child before parent;
    # no existing table/schema/authorization/grant is rewritten by0102.
    if 'cart_context_operations' in actual:
        query('DROP TABLE cart_context_operations;')
    if 'cart_context_runtime' in actual:
        query('DROP TABLE cart_context_runtime;')
    query("DELETE FROM schema_migration_steps WHERE version='0102_cart_context.sql'; DELETE FROM schema_migrations WHERE version='0102_cart_context.sql';")
    require(query('SELECT version,checksum FROM schema_migrations ORDER BY version;') == journal, 'Original migration recovery differs')
    return {'inverse_applied':True,'foreign_keys_kept_on':True,'new_receipts_absent':True,'activated_state_refused':True,'authorization_and_grants_untouched':True}
