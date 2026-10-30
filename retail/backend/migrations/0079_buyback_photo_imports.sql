-- Original metadata/key remains intact; the verified copy is private DB data.
CREATE TABLE buyback_photo_imports (
 photo_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 source_key LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
 snapshot_sha256 BINARY(32) NOT NULL,
 content_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 mime_type VARCHAR(100) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(photo_id) REFERENCES buyback_photos(id)
) ENGINE=InnoDB;
