-- Exact source/target staging evidence; no runtime inventory write grant.
CREATE TABLE migration_inventory_rows (
 source_table VARBINARY(64) NOT NULL,
 identity_sha256 BINARY(32) NOT NULL,
 full_identity LONGBLOB NOT NULL,
 snapshot_sha256 BINARY(32) NOT NULL,
 source_row_sha256 BINARY(32) NOT NULL,
 target_row_sha256 BINARY(32) NOT NULL,
 initial_attempt BINARY(16) NOT NULL,
 policy_version INT NOT NULL CHECK (policy_version=1),
 PRIMARY KEY(source_table,identity_sha256)
) ENGINE=InnoDB;

CREATE TABLE inventory_migration_holds (
 inventory_item_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 location_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 reason VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 snapshot_sha256 BINARY(32) NOT NULL,
 evidence_sha256 BINARY(32) NOT NULL,
 evidence_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK (JSON_VALID(evidence_json)),
 released BOOLEAN NOT NULL DEFAULT FALSE CHECK (released=FALSE),
 policy_version INT NOT NULL CHECK (policy_version=1),
 PRIMARY KEY(inventory_item_id,location_id,reason),
 FOREIGN KEY(inventory_item_id) REFERENCES inventory_items(id),
 CHECK (reason IN ('reservation_mismatch','negative_balance','unmatched_reservations'))
) ENGINE=InnoDB;
