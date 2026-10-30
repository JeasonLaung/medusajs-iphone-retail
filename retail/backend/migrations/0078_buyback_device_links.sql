-- Same independent item/device active uniqueness as the original model.
CREATE TABLE buyback_device_links (
 id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 request_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 item_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 device_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 deleted_at DATETIME(6) NULL,
 active_item VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin GENERATED ALWAYS AS(IF(deleted_at IS NULL,item_id,NULL)) STORED,
 active_device VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin GENERATED ALWAYS AS(IF(deleted_at IS NULL,device_id,NULL)) STORED,
 UNIQUE KEY buyback_one_item(active_item),
 UNIQUE KEY buyback_one_device(active_device),
 INDEX buyback_links_request(request_id,deleted_at),
 FOREIGN KEY(request_id) REFERENCES buyback_requests(id),
 FOREIGN KEY(device_id) REFERENCES devices(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
