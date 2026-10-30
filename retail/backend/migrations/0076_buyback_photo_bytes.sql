-- Private new uploads are committed atomically with their photo record.
-- Legacy storage_key values remain independent and are never exposed.
CREATE TABLE buyback_photo_bytes (
 photo_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
 mime_type VARCHAR(100) NOT NULL,
 sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 content MEDIUMBLOB NOT NULL CHECK(OCTET_LENGTH(content) BETWEEN 3 AND 3145728),
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(photo_id) REFERENCES buyback_photos(id)
) ENGINE=InnoDB;
