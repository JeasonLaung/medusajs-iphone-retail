-- Durable attempt state for new verification intents only. No imported queue
-- activation and no implicit replay of historical mail.
ALTER TABLE customer_verification_mail
 ADD COLUMN attempts INT UNSIGNED NOT NULL DEFAULT 0,
 ADD COLUMN first_attempt_at DATETIME(6) NULL,
 ADD COLUMN next_attempt_at DATETIME(6) NULL,
 ADD COLUMN lease_until DATETIME(6) NULL,
 ADD COLUMN lease_token VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN sender_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN request_payload LONGBLOB NULL,
 ADD COLUMN failed_at DATETIME(6) NULL,
 ADD COLUMN error_code VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD COLUMN sent_at DATETIME(6) NULL,
 ADD COLUMN provider_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NULL,
 ADD INDEX idx_customer_verification_dispatch (status,failed_at,next_attempt_at,lease_until);

CREATE TABLE customer_mail_dispatch_gate (
 id TINYINT PRIMARY KEY CHECK (id=1),
 next_claim_at DATETIME(6) NOT NULL
) ENGINE=InnoDB;
INSERT INTO customer_mail_dispatch_gate VALUES (1,'2000-01-01 00:00:00.000000');
