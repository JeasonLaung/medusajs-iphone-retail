-- New verified-account ownership is explicit and separate from legacy IDs.
-- Only current cart possession plus its paid completion receipt may create it.
CREATE TABLE customer_order_ownership (
 order_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL PRIMARY KEY,
 customer_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 cart_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
 store_id VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 INDEX idx_customer_order_owner (customer_id,created_at,order_id),
 FOREIGN KEY (customer_id) REFERENCES customer_accounts(id)
) ENGINE=InnoDB;
