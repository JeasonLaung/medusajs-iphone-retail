-- New local identities only. Historical subjects and email matches grant nothing.
CREATE TABLE customer_accounts (
 id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 email VARCHAR(254) NOT NULL UNIQUE,
 password_hash VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 first_name VARCHAR(160) NOT NULL,
 last_name VARCHAR(160) NOT NULL,
 phone VARCHAR(64) NOT NULL DEFAULT '',
 enabled BOOLEAN NOT NULL DEFAULT TRUE CHECK (enabled IN (0,1)),
 auth_version BIGINT UNSIGNED NOT NULL DEFAULT 1 CHECK (auth_version > 0),
 verified_at DATETIME(6) NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE customer_verifications (
 id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 customer_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
 token_hash BINARY(32) NOT NULL UNIQUE,
 expires_at DATETIME(6) NOT NULL,
 consumed_at DATETIME(6) NULL,
 created_at DATETIME(6) NOT NULL,
 FOREIGN KEY (customer_id) REFERENCES customer_accounts(id)
) ENGINE=InnoDB;

-- The bearer token exists only inside a row-bound encrypted envelope. A future
-- mail dispatcher must implement delivery/retry before any send is claimed.
CREATE TABLE customer_verification_mail (
 id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 customer_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 payload LONGBLOB NOT NULL,
 status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'pending',
 created_at DATETIME(6) NOT NULL,
 expires_at DATETIME(6) NOT NULL,
 CHECK (status IN ('pending','cancelled','sent')),
 INDEX idx_customer_verification_mail_pending (status, expires_at),
 FOREIGN KEY (customer_id) REFERENCES customer_accounts(id)
) ENGINE=InnoDB;
