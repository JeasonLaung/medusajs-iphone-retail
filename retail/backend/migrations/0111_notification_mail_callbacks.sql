CREATE TABLE notification_mail_callbacks (
 account_id VARCHAR(191) NOT NULL,
 provider VARCHAR(16) NOT NULL,
 event_id VARCHAR(191) NOT NULL,
 body_sha256 BINARY(32) NOT NULL,
 event_type VARCHAR(64) NOT NULL,
 provider_id VARCHAR(191) NOT NULL,
 recipient VARCHAR(254) NOT NULL,
 delivery_status VARCHAR(32) NOT NULL,
 occurred_at DATETIME(6) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY(account_id,provider,event_id),
 INDEX callbacks_submission(account_id,provider_id,occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
ALTER TABLE notification_mail_compositions ADD delivery_status VARCHAR(32) NULL, ADD delivery_at DATETIME(6) NULL, ADD submitted_provider VARCHAR(16) NULL;
ALTER TABLE notification_mail_compositions MODIFY provider_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NULL;
