-- Public device inspection images only; private customer/recycling files are separate.
CREATE TABLE device_photos (
 id CHAR(43) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 operation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
 actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 filename VARCHAR(200) NOT NULL,
 mime_type VARCHAR(20) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 content MEDIUMBLOB NOT NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY(actor_id) REFERENCES admin_users(id),
 CHECK(mime_type IN ('image/jpeg','image/png','image/webp')),
 CHECK(OCTET_LENGTH(content) BETWEEN 3 AND 3145728)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
