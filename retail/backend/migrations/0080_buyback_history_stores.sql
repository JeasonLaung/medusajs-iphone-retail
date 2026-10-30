-- Reviewed historical store scope grants platform read access, never a login.
CREATE TABLE buyback_history_stores (
 request_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 store_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 snapshot_sha256 BINARY(32) NOT NULL,
 FOREIGN KEY(request_id) REFERENCES buyback_requests(id),
 FOREIGN KEY(store_id) REFERENCES commerce_stores(id),
 INDEX buyback_history_store(store_id,request_id)
) ENGINE=InnoDB;
