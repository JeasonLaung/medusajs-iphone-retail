-- Original after_sales_request facts; old customer IDs never grant new ownership.
CREATE TABLE after_sales_requests (
 id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 order_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 customer_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 type VARCHAR(16) NOT NULL CHECK(type IN ('return','exchange','repair')),
 reason LONGTEXT NOT NULL,
 items LONGTEXT NOT NULL CHECK(JSON_VALID(items)),
 status VARCHAR(16) NOT NULL CHECK(status IN ('pending','processing','completed','rejected')),
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 deleted_at DATETIME(6) NULL,
 open_order_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin
 GENERATED ALWAYS AS (CASE WHEN deleted_at IS NULL AND status IN ('pending','processing') THEN order_id ELSE NULL END) STORED,
 UNIQUE KEY one_open_after_sales_request(open_order_id),
 INDEX customer_after_sales_order(order_id,customer_id,created_at)
) ENGINE=InnoDB;
CREATE TABLE customer_after_sales_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO customer_after_sales_runtime VALUES(1,1,FALSE);
CREATE TABLE customer_after_sales_operations (
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 customer_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 order_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
 request_json LONGTEXT NOT NULL CHECK(JSON_VALID(request_json)),
 result_json LONGTEXT NOT NULL CHECK(JSON_VALID(result_json)),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(customer_id) REFERENCES customer_accounts(id),
 FOREIGN KEY(order_id) REFERENCES orders(id),
 FOREIGN KEY(request_id) REFERENCES after_sales_requests(id)
) ENGINE=InnoDB;
