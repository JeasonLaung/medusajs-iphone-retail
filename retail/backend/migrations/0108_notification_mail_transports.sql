-- Explicit mailbox protocol configuration. Old approved Resend accounts retain
-- their original credentials and selection; no account is implicitly enabled.
CREATE TABLE notification_mail_transports (
 account_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 config_envelope LONGBLOB NOT NULL,
 FOREIGN KEY(account_id) REFERENCES notification_mail_accounts(account_id)
) ENGINE=InnoDB;
ALTER TABLE order_mail_deliveries MODIFY provider_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NULL;
ALTER TABLE customer_event_mail_deliveries MODIFY provider_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NULL;
