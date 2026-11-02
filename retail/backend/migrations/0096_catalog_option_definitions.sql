-- Independent shared definitions. All product-option membership mutations
-- take this capability row before locking a product, even while disabled.
INSERT INTO catalog_command_capabilities VALUES('product.option-definitions',1,FALSE);
CREATE TABLE catalog_option_definition_edits (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 option_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 action VARCHAR(20) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 before_json LONGTEXT NOT NULL CHECK(JSON_VALID(before_json)),
 after_json LONGTEXT NOT NULL CHECK(JSON_VALID(after_json)),
 result_revision CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 CHECK(action IN ('create','edit','delete','delete-value')),
 FOREIGN KEY(option_id) REFERENCES catalog_options(id),
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB;
