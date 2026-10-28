-- Anonymous cart draft item mutations. Independent capability defaults closed.
CREATE TABLE cart_item_transition_guard (
 id TINYINT PRIMARY KEY CHECK(id=1),
 preconditions_met BOOLEAN NOT NULL CHECK(preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO cart_item_transition_guard(id,preconditions_met)
SELECT 1,NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE()
 AND table_name IN ('cart_item_runtime','cart_item_operations'));

CREATE TABLE cart_item_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (FALSE,TRUE))
) ENGINE=InnoDB;
INSERT INTO cart_item_runtime VALUES(1,1,FALSE);

CREATE TABLE cart_item_operations (
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 generation CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(generation REGEXP '^[0-9a-f]{64}$'),
 request_sha256 BINARY(32) NOT NULL,
 price_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 price_set_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 result_json LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL CHECK(JSON_VALID(result_json)),
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY(cart_id,operation_id),
 FOREIGN KEY(cart_id) REFERENCES cart_ownership(cart_id)
) ENGINE=InnoDB;

DROP TABLE cart_item_transition_guard;
