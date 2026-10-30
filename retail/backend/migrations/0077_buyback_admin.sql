ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v11;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v12 CHECK(code IN ('authorization.read','authorization.write','buyback.read','buyback.write','device.enroll','device.intake','device.read','device.source.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.after_sales.write','order.allocation.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','price.write','product.read','product.write','warehouse.intake.read','warehouse.intake.write','warehouse.read','warehouse.return.read','warehouse.return.write','warehouse.write'));
INSERT INTO authorization_actions(code) VALUES('buyback.read'),('buyback.write');
UPDATE authorization_runtime_schema SET model_sha256='9219d37bae441485208be46b119ec86dd4ed1189804ee0c5fa7a17d48b268f9f',revision=revision+1 WHERE id=1;
CREATE TABLE buyback_admin_runtime(id TINYINT PRIMARY KEY CHECK(id=1),version INT NOT NULL CHECK(version=1),enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1))) ENGINE=InnoDB;
INSERT INTO buyback_admin_runtime VALUES(1,1,FALSE);
