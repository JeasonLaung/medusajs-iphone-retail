-- New authenticated inbox only. Historical customer_notice identities are not
-- imported or authorized by matching an order email.
CREATE TABLE customer_account_notices (
 id VARCHAR(100) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 customer_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 event_key VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 kind VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 resource_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 reference VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 action VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
 read_at DATETIME(6) NULL,
 created_at DATETIME(6) NOT NULL,
 UNIQUE KEY notice_event(event_key),
 INDEX notice_customer_time(customer_id,created_at,id),
 INDEX notice_customer_unread(customer_id,read_at),
 CHECK(kind IN('shipment.created','delivery.created','order.canceled','after_sales.processing','after_sales.completed','after_sales.rejected','buyback.updated'))
) ENGINE=InnoDB;
