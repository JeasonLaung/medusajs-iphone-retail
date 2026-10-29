-- Explicit separate worker; historical mail and API startup never send it.
CREATE TABLE order_mail_dispatch_gate (
 id TINYINT PRIMARY KEY CHECK(id=1), next_claim_at DATETIME(6) NOT NULL
) ENGINE=InnoDB;
INSERT INTO order_mail_dispatch_gate VALUES(1,'1970-01-01 00:00:00');
CREATE TABLE order_mail_deliveries (
 notification_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 request_envelope LONGBLOB NULL,
 sender_fingerprint CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 first_attempt_at DATETIME(6) NULL,
 next_attempt_at DATETIME(6) NULL,
 lease_until DATETIME(6) NULL,
 lease_token CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 provider_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 error_code VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NULL,
 sent_at DATETIME(6) NULL,
 failed_at DATETIME(6) NULL,
 FOREIGN KEY(notification_id) REFERENCES order_notification_outbox(id)
) ENGINE=InnoDB;
