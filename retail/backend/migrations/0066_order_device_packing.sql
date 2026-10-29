CREATE TABLE order_device_packing_runtime (
 id TINYINT PRIMARY KEY CHECK (id=1),
 version INT NOT NULL CHECK (version=1),
 enabled BOOLEAN NOT NULL CHECK (enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO order_device_packing_runtime VALUES(1,1,FALSE);
CREATE TABLE order_device_packings (
 operation_id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 order_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 fulfillment_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_json LONGTEXT NOT NULL CHECK (JSON_VALID(request_json)),
 before_json LONGTEXT NOT NULL CHECK (JSON_VALID(before_json)),
 after_json LONGTEXT NOT NULL CHECK (JSON_VALID(after_json)),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id),
 FOREIGN KEY(order_id) REFERENCES orders(id),
 FOREIGN KEY(fulfillment_id) REFERENCES fulfillments(id)
) ENGINE=InnoDB;
