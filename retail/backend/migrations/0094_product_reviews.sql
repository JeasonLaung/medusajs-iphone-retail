-- Source review IDs, author snapshots, timestamps and soft deletion are retained.
-- account_id is new-account authority; customer_id alone is historical identity.
CREATE TABLE product_reviews (
 id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL PRIMARY KEY,
 product_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 customer_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 account_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NULL,
 order_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
 verified_purchase BOOLEAN NOT NULL DEFAULT FALSE CHECK(verified_purchase IN(0,1)),
 author TEXT NOT NULL,
 rating INT NOT NULL CHECK(rating BETWEEN 1 AND 5),
 title TEXT NOT NULL,
 content LONGTEXT NOT NULL,
 status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'pending' CHECK(status IN('pending','approved','rejected')),
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 deleted_at DATETIME(6) NULL,
 active_subject VARCHAR(263) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin GENERATED ALWAYS AS (CASE WHEN deleted_at IS NULL THEN CASE WHEN account_id IS NULL THEN CONCAT('legacy:',customer_id) ELSE CONCAT('account:',account_id) END ELSE NULL END) STORED,
 UNIQUE INDEX product_reviews_subject (product_id,active_subject),
 INDEX product_reviews_public (product_id,status,deleted_at,created_at,id),
 INDEX product_reviews_account (account_id,product_id,deleted_at),
 FOREIGN KEY(account_id) REFERENCES customer_accounts(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

-- No mail, stock, payment or fulfillment facts are written by review commands.
-- A command and its review change commit or roll back as one transaction.
CREATE TABLE product_review_commands (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
 command VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(command IN('submit','moderate')),
 actor_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 review_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 previous_status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NULL CHECK(previous_status IN('pending','approved','rejected')),
 result_status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(result_status IN('pending','approved','rejected')),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(review_id) REFERENCES product_reviews(id),
 INDEX product_review_commands_review (review_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE product_review_runtime (
 id TINYINT NOT NULL PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 reads_enabled BOOLEAN NOT NULL DEFAULT FALSE CHECK(reads_enabled IN(0,1)),
 writes_enabled BOOLEAN NOT NULL DEFAULT FALSE CHECK(writes_enabled IN(0,1)),
 CHECK(writes_enabled=FALSE OR reads_enabled=TRUE)
) ENGINE=InnoDB;
INSERT INTO product_review_runtime(id,version,reads_enabled,writes_enabled) VALUES(1,1,FALSE,FALSE);
