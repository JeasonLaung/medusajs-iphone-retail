-- Full-value uniqueness for identities exceeding InnoDB index width.
-- All writers/importers must use the registry in the SAME business transaction.
-- This does not automatically enforce a constraint on the source-mapped tables.
CREATE TABLE unique_key_owners (
  scope VARBINARY(64) NOT NULL,
  owner VARBINARY(764) NOT NULL,
  current_digest BINARY(32) NULL,
  PRIMARY KEY (scope, owner)
) ENGINE=InnoDB;

CREATE TABLE unique_key_buckets (
  scope VARBINARY(64) NOT NULL,
  digest BINARY(32) NOT NULL,
  PRIMARY KEY (scope, digest)
) ENGINE=InnoDB;

CREATE TABLE unique_key_claims (
  scope VARBINARY(64) NOT NULL,
  owner VARBINARY(764) NOT NULL,
  digest BINARY(32) NOT NULL,
  full_value LONGBLOB NOT NULL,
  PRIMARY KEY (scope, owner),
  INDEX lookup_digest (scope, digest),
  FOREIGN KEY (scope, owner) REFERENCES unique_key_owners(scope, owner),
  FOREIGN KEY (scope, digest) REFERENCES unique_key_buckets(scope, digest)
) ENGINE=InnoDB;
