-- Preserve photo records separately from file storage and new account identity.
CREATE TABLE buyback_photos (
 id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 request_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 item_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 photo_type VARCHAR(16) NOT NULL CHECK(photo_type IN ('front','back','edges','damage','packaging','sealed')),
 uploaded_at DATETIME(6) NOT NULL,
 storage_key LONGTEXT NOT NULL,
 mime_type VARCHAR(100) NOT NULL,
 review_status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(review_status IN ('pending','approved','retake','expired')),
 review_note LONGTEXT NOT NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 deleted_at DATETIME(6) NULL,
 INDEX buyback_photo_request_item(request_id,item_id,deleted_at,uploaded_at,id),
 FOREIGN KEY(request_id) REFERENCES buyback_requests(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;
