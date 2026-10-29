ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v5;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v6 CHECK(code IN ('authorization.read','authorization.write','device.enroll','device.read','device.source.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','product.read','product.write','warehouse.read','warehouse.write'));
INSERT INTO authorization_actions(code) VALUES('device.source.write');
UPDATE authorization_runtime_schema SET model_sha256='5d56c72e23d2a495700e1d70719c1449802c94208bd49b6541e8cc963eaa9d45',revision=revision+1 WHERE id=1;
CREATE TABLE device_source_changes (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 store_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json LONGTEXT NOT NULL CHECK(JSON_VALID(request_json)),
 result_json LONGTEXT NOT NULL CHECK(JSON_VALID(result_json)),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
