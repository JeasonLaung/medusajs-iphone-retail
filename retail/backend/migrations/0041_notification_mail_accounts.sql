-- Explicit notification sender approval; importing old accounts never opts them in.
ALTER TABLE authorization_actions DROP CHECK ck_authorization_action_v2;
ALTER TABLE authorization_actions ADD CONSTRAINT ck_authorization_action_v3 CHECK(code IN ('authorization.read','authorization.write','inventory.read','inventory.stock.write','mail.read','mail.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','product.read','product.write'));
INSERT INTO authorization_actions(code) VALUES('mail.read'),('mail.write');
UPDATE authorization_runtime_schema SET model_sha256='d7f0a6b11cefe0238216f4705112a166f433710d19a09ed5db40f69c876dd9b4',revision=revision+1 WHERE id=1;
-- Source-mapped mailbox table previously existed only in offline generated plans.
-- Explicit CREATE deliberately refuses an existing unreviewed table.
CREATE TABLE mail_accounts (
 id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 name LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 email LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 reply_to LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1)),
 api_key LONGBLOB NOT NULL,
 webhook_secret LONGBLOB NOT NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 deleted_at DATETIME(6) NULL
) ENGINE=InnoDB;
CREATE TABLE notification_mail_accounts (
 account_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 has_webhook_secret BOOLEAN NOT NULL CHECK(has_webhook_secret IN (0,1)),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(account_id) REFERENCES mail_accounts(id)
) ENGINE=InnoDB;
CREATE TABLE notification_mail_settings (
 id TINYINT PRIMARY KEY CHECK(id=1),
 revision BIGINT UNSIGNED NOT NULL CHECK(revision>0),
 registration_account_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
 FOREIGN KEY(registration_account_id) REFERENCES notification_mail_accounts(account_id)
) ENGINE=InnoDB;
INSERT INTO notification_mail_settings(id,revision) VALUES(1,1);
CREATE TABLE notification_mail_commands (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_payload LONGBLOB NOT NULL,
 result_revision BIGINT UNSIGNED NOT NULL,
 account_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id),
 FOREIGN KEY(account_id) REFERENCES notification_mail_accounts(account_id)
) ENGINE=InnoDB;
