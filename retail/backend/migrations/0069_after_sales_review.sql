ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v10;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v11 CHECK(code IN ('authorization.read','authorization.write','device.enroll','device.intake','device.read','device.source.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.after_sales.write','order.allocation.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','price.write','product.read','product.write','warehouse.intake.read','warehouse.intake.write','warehouse.read','warehouse.return.read','warehouse.return.write','warehouse.write'));
INSERT INTO authorization_actions(code) VALUES('order.after_sales.write');
UPDATE authorization_runtime_schema SET model_sha256='ab61db8993129bfaead8ac5b6beba9c31412c0cbbdab066b91cbe3ba47e3787a',revision=revision+1 WHERE id=1;
CREATE TABLE after_sales_review_runtime(id TINYINT PRIMARY KEY CHECK(id=1),version INT NOT NULL CHECK(version=1),enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1))) ENGINE=InnoDB;
INSERT INTO after_sales_review_runtime VALUES(1,1,FALSE);
CREATE TABLE after_sales_reviews(
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 order_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json LONGTEXT NOT NULL CHECK(JSON_VALID(request_json)),
 before_status VARCHAR(16) NOT NULL,
 after_status VARCHAR(16) NOT NULL,
 result_json LONGTEXT NOT NULL CHECK(JSON_VALID(result_json)),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id),
 FOREIGN KEY(request_id) REFERENCES after_sales_requests(id)
) ENGINE=InnoDB;
