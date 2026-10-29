-- New location management grants stay explicit; existing roles gain no rights.
ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v3;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v4 CHECK(code IN ('authorization.read','authorization.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','product.read','product.write','warehouse.read','warehouse.write'));
INSERT INTO authorization_actions(code) VALUES('warehouse.read'),('warehouse.write');
UPDATE authorization_runtime_schema SET model_sha256='6b5e8c83320d52e0589f63c1e338dc94fddd7d439580d811c10a33dcfd6731e3',revision=revision+1 WHERE id=1;
CREATE TABLE warehouse_management_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 revision BIGINT UNSIGNED NOT NULL CHECK(revision>0),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO warehouse_management_runtime VALUES(1,1,1,FALSE);
CREATE TABLE warehouse_changes (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 location_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json LONGTEXT NOT NULL CHECK(JSON_VALID(request_json)),
 before_json LONGTEXT NOT NULL CHECK(JSON_VALID(before_json)),
 after_json LONGTEXT NOT NULL CHECK(JSON_VALID(after_json)),
 result_revision BIGINT UNSIGNED NOT NULL CHECK(result_revision>0),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id),
 INDEX idx_warehouse_changes_location(location_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
