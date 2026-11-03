CREATE TABLE notification_mail_sync (
 account_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 source_sha256 BINARY(32) NOT NULL,
 uid_validity BIGINT UNSIGNED NOT NULL DEFAULT 0,
 scanned_uid BIGINT UNSIGNED NOT NULL DEFAULT 0,
 FOREIGN KEY(account_id) REFERENCES notification_mail_accounts(account_id),
 CHECK(uid_validity<=4294967295 AND scanned_uid<=4294967295)
) ENGINE=InnoDB;
CREATE TABLE notification_mail_inbox (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 account_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 source_sha256 BINARY(32) NOT NULL,
 uid_validity BIGINT UNSIGNED NOT NULL,
 uid BIGINT UNSIGNED NOT NULL,
 subject TEXT NOT NULL,
 from_address TEXT NOT NULL,
 received_at DATETIME(6) NOT NULL,
 raw_sha256 BINARY(32) NOT NULL,
 raw_size INT UNSIGNED NOT NULL,
 read_at DATETIME(6) NULL,
 archived BOOLEAN NOT NULL DEFAULT FALSE,
 deleted BOOLEAN NOT NULL DEFAULT FALSE,
 created_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_notification_imap_message(account_id,source_sha256,uid_validity,uid),
 FOREIGN KEY(account_id) REFERENCES notification_mail_accounts(account_id),
 CHECK(uid_validity BETWEEN 1 AND 4294967295 AND uid BETWEEN 1 AND 4294967295),
 CHECK(archived IN(0,1) AND deleted IN(0,1))
) ENGINE=InnoDB;
CREATE TABLE notification_mail_inbox_parts (
 message_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 part_number INT UNSIGNED NOT NULL,
 raw_envelope LONGBLOB NOT NULL,
 PRIMARY KEY(message_id,part_number),
 FOREIGN KEY(message_id) REFERENCES notification_mail_inbox(id)
) ENGINE=InnoDB;
