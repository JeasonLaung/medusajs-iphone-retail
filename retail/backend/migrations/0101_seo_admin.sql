-- Explicit public SEO administration. Existing roles receive no grants.
ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v15;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v16 CHECK(code IN ('authorization.read','authorization.write','buyback.read','buyback.write','device.enroll','device.intake','device.read','device.source.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.after_sales.write','order.allocation.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','order.refund.write','price.read','price.write','product.read','product.write','seo.read','seo.write','store.policies.read','store.policies.write','warehouse.allocation.write','warehouse.intake.read','warehouse.intake.write','warehouse.read','warehouse.return.read','warehouse.return.write','warehouse.shipment.write','warehouse.write'));
INSERT INTO authorization_actions(code) VALUES('seo.read'),('seo.write');
UPDATE authorization_runtime_schema SET model_sha256='b640b0796228b6e305d09e2979735b58ab7b3b28c474e4a36d1b32e1a197e47e',revision=revision+1 WHERE id=1;
-- One exact global lock serializes all full-text key/path writes without
-- truncating source keys or imposing a prefix/collation uniqueness shortcut.
CREATE TABLE seo_command_state (
 id INT NOT NULL PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO seo_command_state VALUES(1,1,FALSE);
CREATE TABLE seo_command_receipts (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 resource VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(resource IN ('settings','pages')),
 action VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(action IN ('save','delete')),
 target_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 before_revision CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 result_revision CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB;
