-- Original historical fields stay independent of new login identities.
CREATE TABLE buyback_requests (
 id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 customer_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 submission_key LONGTEXT NOT NULL,
 contact_name LONGTEXT NOT NULL,
 email LONGTEXT NOT NULL,
 phone LONGTEXT NOT NULL,
 return_address LONGTEXT NOT NULL CHECK(JSON_VALID(return_address)),
 items LONGTEXT NOT NULL CHECK(JSON_VALID(items)),
 status VARCHAR(32) NOT NULL CHECK(status IN ('awaiting_photos','photo_review','ready_to_ship','in_transit','received','final_offer','accepted','paid','return_requested','returning','returned','canceled')),
 reference_amount DECIMAL(38,18) NULL,
 final_amount DECIMAL(38,18) NULL,
 currency_code VARCHAR(16) NOT NULL,
 shipping_instructions LONGTEXT NOT NULL,
 tracking_number LONGTEXT NOT NULL,
 return_tracking LONGTEXT NOT NULL,
 payment_reference LONGTEXT NOT NULL,
 review_note LONGTEXT NOT NULL,
 history LONGTEXT NOT NULL CHECK(JSON_VALID(history)),
 closed_at DATETIME(6) NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 deleted_at DATETIME(6) NULL,
 raw_reference_amount LONGTEXT NULL CHECK(raw_reference_amount IS NULL OR JSON_VALID(raw_reference_amount)),
 raw_final_amount LONGTEXT NULL CHECK(raw_final_amount IS NULL OR JSON_VALID(raw_final_amount)),
 active_submission_digest BINARY(32) GENERATED ALWAYS AS (IF(deleted_at IS NULL,UNHEX(SHA2(submission_key,256)),NULL)) STORED,
 UNIQUE KEY buyback_customer_submission(customer_id,active_submission_digest),
 INDEX buyback_customer_time(customer_id,deleted_at,created_at,id),
 INDEX buyback_status_time(status,deleted_at,created_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

-- New ownership requires an explicit binding; matching old emails/IDs grants none.
CREATE TABLE customer_buyback_submissions (
 customer_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 submission_key CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 store_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 request_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL UNIQUE,
 request_json LONGTEXT NOT NULL CHECK(JSON_VALID(request_json)),
 result_json LONGTEXT NOT NULL CHECK(JSON_VALID(result_json)),
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY(customer_id,submission_key),
 FOREIGN KEY(customer_id) REFERENCES customer_accounts(id),
 FOREIGN KEY(store_id) REFERENCES commerce_stores(id),
 FOREIGN KEY(request_id) REFERENCES buyback_requests(id)
) ENGINE=InnoDB;
CREATE TABLE buyback_request_runtime (
 id TINYINT PRIMARY KEY CHECK(id=1),
 version INT NOT NULL CHECK(version=1),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO buyback_request_runtime VALUES(1,1,FALSE);
