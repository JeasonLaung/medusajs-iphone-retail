-- No startup DDL. Apply explicitly in the isolated target alongside the
-- reviewed relational mail schema. This ledger never grants delivery rights.
CREATE TABLE migration_mail_dispositions (
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
  CONSTRAINT ck_migration_mail_source CHECK (source_table = _binary'mail_message'),
  CONSTRAINT ck_migration_mail_version CHECK (policy_version = 1),
  CONSTRAINT ck_migration_mail_disposition CHECK (
    (disposition = _binary'suppress-permanently' AND reason = _binary'excluded-legacy-registration-capability')
    OR (disposition = _binary'hold' AND reason = _binary'legacy-delivery-reconciliation')
  )
) ENGINE=InnoDB;
