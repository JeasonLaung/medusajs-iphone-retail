-- New local accounts only. Every password/role/permission/enable-state update
-- must advance this generation in the same transaction; no legacy accounts.
ALTER TABLE admin_users ADD COLUMN auth_version BIGINT UNSIGNED NOT NULL DEFAULT 1;
ALTER TABLE admin_users ADD CONSTRAINT chk_admin_auth_version CHECK (auth_version > 0);
