ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v8;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v9 CHECK(code IN ('authorization.read','authorization.write','device.enroll','device.intake','device.read','device.source.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','price.write','product.read','product.write','warehouse.intake.read','warehouse.intake.write','warehouse.read','warehouse.return.read','warehouse.return.write','warehouse.write'));
INSERT INTO authorization_actions(code) VALUES('price.write');
UPDATE authorization_runtime_schema SET model_sha256='1b974d153bffcdd42f27cd8d771a68f03f7fcfd7992fcc6c63145baf9ff9a18e',revision=revision+1 WHERE id=1;
CREATE TABLE catalog_price_edit_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1)),
 revision BIGINT UNSIGNED NOT NULL CHECK(revision>0)
) ENGINE=InnoDB;
INSERT INTO catalog_price_edit_runtime VALUES(1,1,FALSE,1);
CREATE TABLE catalog_price_edits (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 product_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 variant_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 price_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json LONGTEXT NOT NULL CHECK(JSON_VALID(request_json)),
 before_json LONGTEXT NOT NULL CHECK(JSON_VALID(before_json)),
 after_json LONGTEXT NOT NULL CHECK(JSON_VALID(after_json)),
 result_revision CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
