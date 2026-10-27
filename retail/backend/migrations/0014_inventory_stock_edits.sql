-- Specific audited stock command only. Reservation/generic writes stay disabled.
CREATE TABLE inventory_stock_edits (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 level_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK (JSON_VALID(request_json)),
 before_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK (JSON_VALID(before_json)),
 after_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK (JSON_VALID(after_json)),
 result_revision CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 INDEX idx_inventory_stock_edit_level (level_id,created_at),
 FOREIGN KEY (actor_id) REFERENCES admin_users(id),
 FOREIGN KEY (level_id) REFERENCES inventory_levels(id)
) ENGINE=InnoDB;
CREATE TABLE inventory_command_capabilities (
 command VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 version INT NOT NULL CHECK (version=1),
 enabled BOOLEAN NOT NULL CHECK (enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO inventory_command_capabilities VALUES ('inventory.edit_stock',1,TRUE);
