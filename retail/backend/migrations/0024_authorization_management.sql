-- Existing grants stay explicit; role names do not gain new management rights.
ALTER TABLE authorization_actions DROP CHECK authorization_actions_chk_1;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v2 CHECK(code IN ('authorization.read','authorization.write','inventory.read','inventory.stock.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','product.read','product.write'));
INSERT INTO authorization_actions(code) VALUES('authorization.read'),('authorization.write');
UPDATE authorization_runtime_schema SET model_sha256='f0d769d4968f7e8cce49f92b064c8bc557dfb4ab6b18642dd6f1421f0dfc1309',revision=revision+1 WHERE id=1;
CREATE TABLE authorization_changes (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK(JSON_VALID(request_json)),
 before_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK(JSON_VALID(before_json)),
 after_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK(JSON_VALID(after_json)),
 result_revision BIGINT UNSIGNED NOT NULL CHECK(result_revision>0),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id)
) ENGINE=InnoDB;
