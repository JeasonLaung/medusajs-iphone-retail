-- Business-subject provenance only; old account binding is permanently excluded.
CREATE TABLE migration_customer_dispositions (
 source_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 snapshot_sha256 BINARY(32) NOT NULL,
 source_row_sha256 BINARY(32) NOT NULL,
 target_row_sha256 BINARY(32) NOT NULL,
 initial_attempt BINARY(16) NOT NULL,
 policy_version INT NOT NULL CHECK (policy_version=1),
 disposition VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL
   CHECK (disposition='excluded'),
 reason VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL
   CHECK (reason='historical-subject-not-login')
) ENGINE=InnoDB;
