-- Internal transaction receipts. No HTTP or general promotion write capability.
-- References deliberately do not cascade: history must survive configuration deletion.
CREATE TABLE promotion_usage_operations (
 scope_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_sha256 BINARY(32) NOT NULL,
 state VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(state IN ('pending','registered','reverted')),
 deltas_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK(JSON_VALID(deltas_json)),
 result_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK(JSON_VALID(result_json)),
 created_at DATETIME(6) NOT NULL,
 registered_at DATETIME(6) NULL,
 reverted_at DATETIME(6) NULL,
 PRIMARY KEY(scope_id,operation_id)
) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;
