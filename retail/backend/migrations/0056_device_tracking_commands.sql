-- Explicit new device rights; existing roles receive no implicit grants.
ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v4;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v5 CHECK(code IN ('authorization.read','authorization.write','device.enroll','device.read','inventory.read','inventory.stock.write','mail.read','mail.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','product.read','product.write','warehouse.read','warehouse.write'));
INSERT INTO authorization_actions(code) VALUES('device.enroll'),('device.read');
UPDATE authorization_runtime_schema SET model_sha256='0e335ef833eb2c5b4290b8606b3f26d62ef638bf13a7f09e8bab5dc03c7ed36b',revision=revision+1 WHERE id=1;
CREATE TABLE device_tracking_changes (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 product_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 variant_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json LONGTEXT NOT NULL CHECK(JSON_VALID(request_json)),
 result_json LONGTEXT NOT NULL CHECK(JSON_VALID(result_json)),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
