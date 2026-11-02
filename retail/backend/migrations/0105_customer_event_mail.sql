-- Event identity is independent of the coalesced/readable in-app inbox.
-- No historical backfill and no API-triggered network calls.
CREATE TABLE customer_event_mail (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 customer_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 event_key VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
 kind VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 resource_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 reference VARCHAR(191) NOT NULL,
 action VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'pending' CHECK(status IN('pending','sending','sent','failed')),
 attempts INT UNSIGNED NOT NULL DEFAULT 0,
 created_at DATETIME(6) NOT NULL,
 INDEX customer_event_mail_queue(status,created_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE customer_event_mail_gate (
 id TINYINT PRIMARY KEY CHECK(id=1), next_claim_at DATETIME(6) NOT NULL
) ENGINE=InnoDB;
INSERT INTO customer_event_mail_gate VALUES(1,'1970-01-01 00:00:00');
CREATE TABLE customer_event_mail_deliveries (
 notification_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 request_envelope LONGBLOB NULL,
 recipient_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 sender_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 first_attempt_at DATETIME(6) NULL,
 next_attempt_at DATETIME(6) NULL,
 lease_until DATETIME(6) NULL,
 lease_token CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 provider_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 error_code VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
 sent_at DATETIME(6) NULL,
 failed_at DATETIME(6) NULL,
 FOREIGN KEY(notification_id) REFERENCES customer_event_mail(id)
) ENGINE=InnoDB;
