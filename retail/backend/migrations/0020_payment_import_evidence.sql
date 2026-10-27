-- Exact historical payment evidence, not a checkout or account-ownership grant.
CREATE TABLE migration_payment_rows (
 source_table VARBINARY(64) NOT NULL,
 identity_sha256 BINARY(32) NOT NULL,
 full_identity LONGBLOB NOT NULL,
 snapshot_sha256 BINARY(32) NOT NULL,
 source_row_sha256 BINARY(32) NOT NULL,
 target_row_sha256 BINARY(32) NULL,
 CONSTRAINT ck_payment_receipt_protection CHECK (
  (source_table IN (_binary'account_holder',_binary'payment',_binary'payment_session') AND target_row_sha256 IS NULL)
  OR (source_table IN (_binary'payment_collection',_binary'payment_collection_payment_providers',_binary'capture',_binary'refund',_binary'refund_reason') AND target_row_sha256 IS NOT NULL)
 ),
 initial_attempt BINARY(16) NOT NULL,
 policy_version INT NOT NULL CHECK (policy_version=1),
 PRIMARY KEY(source_table,identity_sha256)
) ENGINE=InnoDB;

CREATE TABLE payment_migration_reference_holds (
 source_table VARBINARY(64) NOT NULL,
 identity_sha256 BINARY(32) NOT NULL,
 column_name VARBINARY(64) NOT NULL,
 full_identity LONGBLOB NOT NULL,
 parent_table VARBINARY(64) NOT NULL,
 parent_column VARBINARY(64) NOT NULL,
 parent_identity VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 reason VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL
   CHECK (reason IN ('parent_domain_unavailable','missing_parent')),
 snapshot_sha256 BINARY(32) NOT NULL,
 source_row_sha256 BINARY(32) NOT NULL,
 released BOOLEAN NOT NULL DEFAULT FALSE CHECK (released=FALSE),
 policy_version INT NOT NULL CHECK (policy_version=1),
 PRIMARY KEY(source_table,identity_sha256,column_name),
 FOREIGN KEY(source_table,identity_sha256) REFERENCES migration_payment_rows(source_table,identity_sha256)
) ENGINE=InnoDB;
