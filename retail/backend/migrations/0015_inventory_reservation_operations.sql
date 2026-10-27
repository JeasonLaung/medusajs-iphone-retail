-- Internal transaction participant; no public command until order integration.
CREATE TABLE inventory_reservation_operations (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 owner_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 actor_ref VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 kind VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK (kind IN ('reserve','release')),
 request_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK (JSON_VALID(request_json)),
 result_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK (JSON_VALID(result_json)),
 before_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK (JSON_VALID(before_json)),
 after_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK (JSON_VALID(after_json)),
 created_at DATETIME(6) NOT NULL,
 INDEX idx_inventory_reservation_owner (owner_id,created_at)
) ENGINE=InnoDB;
INSERT INTO inventory_command_capabilities VALUES ('inventory.reservations',1,FALSE);
