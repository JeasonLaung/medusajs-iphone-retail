-- Explicit isolated-target migration. This ledger never grants account access.
CREATE TABLE migration_holder_dispositions (
  source_id VARBINARY(764) NOT NULL PRIMARY KEY,
  snapshot_sha256 BINARY(32) NOT NULL,
  source_row_sha256 BINARY(32) NOT NULL,
  target_row_sha256 BINARY(32) NOT NULL,
  initial_attempt BINARY(16) NOT NULL,
  policy_version INT NOT NULL,
  disposition VARBINARY(32) NOT NULL,
  reason VARBINARY(64) NOT NULL,
  CONSTRAINT ck_migration_holder_version CHECK (policy_version = 1),
  CONSTRAINT ck_migration_holder_disposition CHECK (
    disposition = _binary'hold' AND reason = _binary'legacy-account-holder-reconciliation'
  )
) ENGINE=InnoDB;
