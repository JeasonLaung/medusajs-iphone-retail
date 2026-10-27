-- Explicit staging evidence for reviewed catalog/configuration rows.
CREATE TABLE migration_catalog_rows (
  source_table VARBINARY(64) NOT NULL,
  identity_sha256 BINARY(32) NOT NULL,
  full_identity LONGBLOB NOT NULL,
  snapshot_sha256 BINARY(32) NOT NULL,
  source_row_sha256 BINARY(32) NOT NULL,
  target_row_sha256 BINARY(32) NOT NULL,
  initial_attempt BINARY(16) NOT NULL,
  policy_version INT NOT NULL,
  PRIMARY KEY (source_table,identity_sha256),
  CONSTRAINT ck_migration_catalog_version CHECK (policy_version=1)
) ENGINE=InnoDB;
