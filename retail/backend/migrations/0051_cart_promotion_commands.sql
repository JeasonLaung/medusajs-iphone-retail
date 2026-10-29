-- Explicit checkout command capability; enabling a draft does not authorize
-- altering an existing provider payment or registering promotion budget usage.
CREATE TABLE cart_promotion_command_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (FALSE,TRUE))
) ENGINE=InnoDB;
INSERT INTO cart_promotion_command_runtime VALUES(1,1,FALSE);

CREATE TABLE cart_promotion_command_operations (
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 generation CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_sha256 BINARY(32) NOT NULL,
 result_version BIGINT UNSIGNED NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY(cart_id,operation_id),
 CONSTRAINT fk_cart_promotion_command_owner FOREIGN KEY(cart_id) REFERENCES cart_ownership(cart_id)
) ENGINE=InnoDB;
