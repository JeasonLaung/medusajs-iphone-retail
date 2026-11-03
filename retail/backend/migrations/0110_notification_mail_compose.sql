CREATE TABLE notification_mail_compositions (
 id CHAR(36) PRIMARY KEY,
 account_id VARCHAR(255) NOT NULL,
 revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
 subject VARCHAR(1024) NOT NULL,
 content_envelope MEDIUMBLOB NOT NULL,
 status VARCHAR(32) NOT NULL DEFAULT 'draft',
 provider_id VARCHAR(191) NULL,
 error_code VARCHAR(64) NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 sent_at DATETIME(6) NULL,
 INDEX compositions_account(account_id,status,updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
