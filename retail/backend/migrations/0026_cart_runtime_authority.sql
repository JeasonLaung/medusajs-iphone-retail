-- New runtime authority only: never backfill from historical cart/customer IDs.
CREATE TABLE cart_authority_transition_guard (
 id TINYINT PRIMARY KEY CHECK(id=1),
 preconditions_met BOOLEAN NOT NULL CHECK(preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO cart_authority_transition_guard(id,preconditions_met)
SELECT 1,NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE()
 AND table_name IN ('cart_command_capabilities','cart_ownership','cart_creation_scopes','cart_creation_operations'));

CREATE TABLE cart_command_capabilities (
 command VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY CHECK(command IN ('cart.create','cart.access')),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (FALSE,TRUE))
) ENGINE=InnoDB;
INSERT INTO cart_command_capabilities VALUES ('cart.create',1,FALSE),('cart.access',1,FALSE);

CREATE TABLE cart_ownership (
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 store_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 generation CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(generation REGEXP '^[0-9a-f]{64}$'),
 version BIGINT UNSIGNED NOT NULL CHECK(version>0),
 active BOOLEAN NOT NULL CHECK(active IN (FALSE,TRUE)),
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 FOREIGN KEY(cart_id) REFERENCES carts(id),
 FOREIGN KEY(store_id) REFERENCES commerce_stores(id)
) ENGINE=InnoDB;

-- Hash of a separately authenticated, server-created anonymous scope. This is
-- not a cart ID, an email, a source customer ID or a raw bearer credential.
CREATE TABLE cart_creation_scopes (
 scope_sha256 BINARY(32) PRIMARY KEY,
 created_at DATETIME(6) NOT NULL
) ENGINE=InnoDB;

CREATE TABLE cart_creation_operations (
 scope_sha256 BINARY(32) NOT NULL,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_sha256 BINARY(32) NOT NULL,
 cart_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 generation CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(generation REGEXP '^[0-9a-f]{64}$'),
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY(scope_sha256,operation_id),
 FOREIGN KEY(scope_sha256) REFERENCES cart_creation_scopes(scope_sha256),
 FOREIGN KEY(cart_id) REFERENCES cart_ownership(cart_id)
) ENGINE=InnoDB;
