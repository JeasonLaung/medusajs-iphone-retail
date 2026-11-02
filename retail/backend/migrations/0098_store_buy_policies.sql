-- Explicit store policy authority; existing roles receive no new grants.
ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v14;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v15 CHECK(code IN ('authorization.read','authorization.write','buyback.read','buyback.write','device.enroll','device.intake','device.read','device.source.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.after_sales.write','order.allocation.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','order.refund.write','price.read','price.write','product.read','product.write','store.policies.read','store.policies.write','warehouse.allocation.write','warehouse.intake.read','warehouse.intake.write','warehouse.read','warehouse.return.read','warehouse.return.write','warehouse.shipment.write','warehouse.write'));
INSERT INTO authorization_actions(code) VALUES('store.policies.read'),('store.policies.write');
UPDATE authorization_runtime_schema SET model_sha256='ea9312b08cfd0dfc0a1cf068daa79cc72379334592e6de5f430989190b93fac1',revision=revision+1 WHERE id=1;
INSERT INTO catalog_command_capabilities VALUES('store.buy-policies',1,FALSE);
CREATE TABLE store_policy_edits (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 store_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 before_json LONGTEXT NOT NULL CHECK(JSON_VALID(before_json)),
 after_json LONGTEXT NOT NULL CHECK(JSON_VALID(after_json)),
 result_revision CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(store_id) REFERENCES commerce_stores(id),
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB;
