-- Durable untrusted notifications. A notification is never payment evidence.
CREATE TABLE mollie_notification_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (FALSE,TRUE))
) ENGINE=InnoDB;
INSERT INTO mollie_notification_runtime VALUES(1,1,FALSE);
CREATE TABLE mollie_payment_references (
 provider_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 remote_id VARCHAR(124) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 scope_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 effect_kind VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(effect_kind='create_session'),
 resource_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_sha256 BINARY(32) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY(provider_id,remote_id),
 UNIQUE KEY local_session(provider_id,resource_id),
 FOREIGN KEY(scope_id,operation_id,effect_kind,resource_id)
  REFERENCES payment_effect_journal(scope_id,operation_id,effect_kind,resource_id)
) ENGINE=InnoDB;
CREATE TABLE mollie_notification_inbox (
 provider_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 remote_id VARCHAR(124) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 generation BIGINT UNSIGNED NOT NULL CHECK(generation>0),
 processed_generation BIGINT UNSIGNED NOT NULL DEFAULT 0,
 claimed_generation BIGINT UNSIGNED NOT NULL DEFAULT 0,
 state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(state IN ('pending','inflight','idle')),
 attempt_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 lease_until DATETIME(6) NULL,
 available_at DATETIME(6) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 PRIMARY KEY(provider_id,remote_id),
 INDEX due_notifications(state,available_at,lease_until),
 FOREIGN KEY(provider_id,remote_id) REFERENCES mollie_payment_references(provider_id,remote_id),
 CHECK(processed_generation<=claimed_generation AND claimed_generation<=generation),
 CHECK((state='inflight' AND attempt_id IS NOT NULL AND lease_until IS NOT NULL AND claimed_generation>processed_generation)
    OR (state<>'inflight' AND lease_until IS NULL)),
 CHECK((state='idle' AND processed_generation=generation) OR state<>'idle')
) ENGINE=InnoDB;
CREATE TABLE mollie_notification_observations (
 attempt_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 provider_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 remote_id VARCHAR(124) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 generation BIGINT UNSIGNED NOT NULL CHECK(generation>0),
 result_envelope LONGBLOB NOT NULL CHECK(OCTET_LENGTH(result_envelope) BETWEEN 34 AND 1048673),
 created_at DATETIME(6) NOT NULL,
 consumed_at DATETIME(6) NULL,
 INDEX unconsumed(consumed_at,created_at),
 FOREIGN KEY(provider_id,remote_id) REFERENCES mollie_payment_references(provider_id,remote_id)
) ENGINE=InnoDB;
