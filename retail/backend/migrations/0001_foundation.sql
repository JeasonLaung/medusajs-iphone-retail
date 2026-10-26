CREATE TABLE IF NOT EXISTS admin_users (
  id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  email VARCHAR(254) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  name VARCHAR(160) NOT NULL DEFAULT '',
  role VARCHAR(32) NOT NULL DEFAULT 'admin',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
CREATE TABLE IF NOT EXISTS admin_sessions (
  token_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  user_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  expires_at DATETIME(6) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  INDEX idx_admin_session_user (user_id),
  INDEX idx_admin_session_expiry (expires_at),
  FOREIGN KEY (user_id) REFERENCES admin_users(id)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS audit_events (
  id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  actor_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  action VARCHAR(100) NOT NULL,
  resource_id VARCHAR(100) NOT NULL,
  data JSON NOT NULL,
  created_at DATETIME(6) NOT NULL,
  INDEX idx_audit_resource (resource_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS catalog_products (
  id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  handle VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL UNIQUE,
  title VARCHAR(500) NOT NULL,
  status VARCHAR(32) NOT NULL,
  thumbnail TEXT,
  collection_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin,
  payload JSON NOT NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  deleted_at DATETIME(6),
  INDEX idx_product_status (status, deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS catalog_variants (
  id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  product_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  sku VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin,
  title VARCHAR(500) NOT NULL,
  payload JSON NOT NULL,
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  deleted_at DATETIME(6),
  INDEX idx_variant_product (product_id, deleted_at),
  INDEX idx_variant_sku (sku),
  FOREIGN KEY (product_id) REFERENCES catalog_products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS catalog_prices (
  id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  variant_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  currency_code CHAR(3) CHARACTER SET ascii NOT NULL,
  amount DECIMAL(24,8) NOT NULL,
  region_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin,
  price_list_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin,
  min_quantity DECIMAL(24,8),
  max_quantity DECIMAL(24,8),
  payload JSON NOT NULL,
  deleted_at DATETIME(6),
  INDEX idx_price_variant (variant_id,currency_code),
  FOREIGN KEY (variant_id) REFERENCES catalog_variants(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS catalog_categories (
  id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  handle VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL UNIQUE,
  name VARCHAR(255) NOT NULL,
  parent_category_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin,
  is_active BOOLEAN NOT NULL,
  is_internal BOOLEAN NOT NULL,
  payload JSON NOT NULL,
  deleted_at DATETIME(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS catalog_category_products (
  product_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  category_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  PRIMARY KEY (product_id, category_id),
  FOREIGN KEY (product_id) REFERENCES catalog_products(id),
  FOREIGN KEY (category_id) REFERENCES catalog_categories(id)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS catalog_collections (
  id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  handle VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL UNIQUE,
  title VARCHAR(255) NOT NULL,
  payload JSON NOT NULL,
  deleted_at DATETIME(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS commerce_regions (
  id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  currency_code CHAR(3) CHARACTER SET ascii NOT NULL,
  payload JSON NOT NULL,
  deleted_at DATETIME(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS migration_runs (
  id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  bundle_sha256 CHAR(64) CHARACTER SET ascii NOT NULL UNIQUE,
  source_commit CHAR(40) CHARACTER SET ascii NOT NULL,
  counts JSON NOT NULL,
  created_at DATETIME(6) NOT NULL
) ENGINE=InnoDB;
