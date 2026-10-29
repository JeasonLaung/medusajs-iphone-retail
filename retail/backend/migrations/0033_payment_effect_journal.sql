-- Private external-effect receipts. No provider calls or public payment writes.
-- No cascading references: recovery evidence must survive source soft deletion.
CREATE TABLE payment_effect_journal (
 scope_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 effect_kind VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(effect_kind IN ('delete_session','create_session')),
 resource_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_sha256 BINARY(32) NOT NULL,
 state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(state IN ('pending','inflight','succeeded','failed','uncertain')),
 attempt_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 attempts INT NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 100),
 lease_until DATETIME(6) NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 PRIMARY KEY(scope_id,operation_id,effect_kind,resource_id),
 INDEX recovery(state,lease_until),
 CHECK((state='pending' AND attempt_id IS NULL AND attempts=0 AND lease_until IS NULL)
    OR (state<>'pending' AND attempt_id IS NOT NULL AND attempts>0 AND lease_until IS NOT NULL))
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;
