-- Group definitions and memberships belong to newly verified accounts only.
-- No historical customer IDs, emails or credentials are adopted.
CREATE TABLE customer_group_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1), version INT NOT NULL CHECK(version=1),
 revision BIGINT UNSIGNED NOT NULL CHECK(revision>0), enabled BOOLEAN NOT NULL
) ENGINE=InnoDB;
INSERT INTO customer_group_runtime VALUES(1,1,1,FALSE);
CREATE TABLE customer_groups (
 id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 name VARCHAR(255) NOT NULL, created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL, deleted_at DATETIME(6) NULL
) ENGINE=InnoDB;
CREATE TABLE customer_group_members (
 group_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 customer_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY(group_id,customer_id), INDEX customer_memberships(customer_id,group_id),
 FOREIGN KEY(group_id) REFERENCES customer_groups(id),
 FOREIGN KEY(customer_id) REFERENCES customer_accounts(id)
) ENGINE=InnoDB;
CREATE TABLE customer_group_commands (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 group_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json JSON NOT NULL, result_json JSON NOT NULL, created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id), FOREIGN KEY(group_id) REFERENCES customer_groups(id)
) ENGINE=InnoDB;
