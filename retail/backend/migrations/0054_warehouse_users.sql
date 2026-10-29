CREATE TABLE warehouse_user_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO warehouse_user_runtime VALUES(1,1,FALSE);
-- Only accounts created by this independent lifecycle may be managed here.
CREATE TABLE warehouse_user_accounts (
 user_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 role_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
 FOREIGN KEY(user_id) REFERENCES authorization_warehouse_scopes(user_id),
 FOREIGN KEY(role_id) REFERENCES authorization_roles(id)
) ENGINE=InnoDB;
CREATE TABLE warehouse_user_changes (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 user_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_json LONGTEXT NOT NULL CHECK(JSON_VALID(request_json)),
 password_hash VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 before_json LONGTEXT NOT NULL CHECK(JSON_VALID(before_json)),
 after_json LONGTEXT NOT NULL CHECK(JSON_VALID(after_json)),
 result_revision BIGINT UNSIGNED NOT NULL CHECK(result_revision>0),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id),
 FOREIGN KEY(user_id) REFERENCES warehouse_user_accounts(user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
