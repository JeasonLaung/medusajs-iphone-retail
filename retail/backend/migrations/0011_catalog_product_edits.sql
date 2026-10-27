-- A narrowly scoped command capability; generic catalog writes stay disabled.
CREATE TABLE catalog_product_edits (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 product_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK (JSON_VALID(request_json)),
 before_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK (JSON_VALID(before_json)),
 after_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK (JSON_VALID(after_json)),
 result_revision CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 INDEX idx_catalog_edit_product (product_id,created_at),
 FOREIGN KEY (actor_id) REFERENCES admin_users(id),
 FOREIGN KEY (product_id) REFERENCES catalog_products(id)
) ENGINE=InnoDB;
CREATE TABLE catalog_command_capabilities (
 command VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 version INT NOT NULL CHECK (version=1),
 enabled BOOLEAN NOT NULL CHECK (enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO catalog_command_capabilities VALUES ('product.edit_basics',1,TRUE);
