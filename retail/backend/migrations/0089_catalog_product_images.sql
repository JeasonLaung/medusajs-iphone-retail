-- Public catalog assets are independent from private device/buyback photos.
INSERT INTO catalog_command_capabilities VALUES('product.images',1,FALSE);
CREATE TABLE catalog_image_assets (
 image_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 filename VARCHAR(200) NOT NULL,
 mime_type VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 content MEDIUMBLOB NOT NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(image_id) REFERENCES catalog_images(id),
 CHECK(mime_type IN ('image/jpeg','image/png','image/webp')),
 CHECK(OCTET_LENGTH(content)>0 AND OCTET_LENGTH(content)<=3145728)
) ENGINE=InnoDB;
CREATE TABLE catalog_image_edits (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 product_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 before_json LONGTEXT NOT NULL CHECK(JSON_VALID(before_json)),
 after_json LONGTEXT NOT NULL CHECK(JSON_VALID(after_json)),
 result_revision CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(product_id) REFERENCES catalog_products(id),
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB;
