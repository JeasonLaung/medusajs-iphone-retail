-- New local accounts only; neither legacy metadata nor an empty scope means all.
CREATE TABLE warehouse_scope_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO warehouse_scope_runtime VALUES(1,1,FALSE);
CREATE TABLE authorization_warehouse_scopes (
 user_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 mode VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(mode IN ('selected','all')),
 FOREIGN KEY(user_id) REFERENCES authorization_principals(user_id)
) ENGINE=InnoDB;
CREATE TABLE authorization_warehouse_locations (
 user_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 location_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 PRIMARY KEY(user_id,location_id),
 FOREIGN KEY(user_id) REFERENCES authorization_warehouse_scopes(user_id),
 FOREIGN KEY(location_id) REFERENCES inventory_locations(id)
) ENGINE=InnoDB;
