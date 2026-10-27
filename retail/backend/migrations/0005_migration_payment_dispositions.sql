-- Explicit isolated-target migration. No payment permission can be granted
-- here; releasing historical payments requires a separate reconciled design.
CREATE TABLE migration_payment_dispositions (
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
  CONSTRAINT ck_migration_payment_source CHECK (source_table IN (_binary'payment_session', _binary'payment')),
  CONSTRAINT ck_migration_payment_version CHECK (policy_version = 1),
  CONSTRAINT ck_migration_payment_disposition CHECK (
    disposition = _binary'hold' AND reason = _binary'legacy-payment-reconciliation'
  )
) ENGINE=InnoDB;
