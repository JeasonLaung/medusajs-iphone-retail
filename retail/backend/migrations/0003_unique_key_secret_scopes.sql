-- Authentication of an index-key configuration prevents using the wrong
-- HMAC material/key ID to search a different bucket and silently miss claims.
-- Verification bytes are an authenticated envelope, never the index key.
CREATE TABLE unique_key_secret_scopes (
  scope VARBINARY(64) NOT NULL PRIMARY KEY,
  key_id VARBINARY(64) NOT NULL,
  verification LONGBLOB NOT NULL
) ENGINE=InnoDB;
