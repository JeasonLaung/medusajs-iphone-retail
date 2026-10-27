-- Explicit isolated-target migration. Configuration hold is not authorization.
CREATE TABLE migration_merchant_dispositions (
  source_table VARBINARY(64) NOT NULL,
  source_id VARBINARY(764) NOT NULL,
  snapshot_sha256 BINARY(32) NOT NULL,
  source_row_sha256 BINARY(32) NOT NULL,
  target_row_sha256 BINARY(32) NOT NULL,
  initial_attempt BINARY(16) NOT NULL,
  policy_version INT NOT NULL,
  disposition VARBINARY(32) NOT NULL,
  reason VARBINARY(64) NOT NULL,
  PRIMARY KEY (source_table, source_id),
  CONSTRAINT ck_migration_merchant_source CHECK (source_table IN (
    _binary'mail_account', _binary'seo_provider', _binary'seo_setting',
    _binary'webhook_endpoint', _binary'mollie_config', _binary'paypal_connection', _binary'paypal_settings'
  )),
  CONSTRAINT ck_migration_merchant_version CHECK (policy_version = 1),
  CONSTRAINT ck_migration_merchant_disposition CHECK (
    disposition = _binary'hold' AND reason = _binary'legacy-merchant-reconciliation'
  )
) ENGINE=InnoDB;
