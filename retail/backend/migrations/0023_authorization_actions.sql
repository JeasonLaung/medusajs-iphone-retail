-- New accounts receive no action grants from their legacy role label.
CREATE TABLE authorization_runtime_schema (
 id TINYINT PRIMARY KEY CHECK(id=1),
 schema_version INT NOT NULL CHECK(schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 revision BIGINT UNSIGNED NOT NULL CHECK(revision>0),
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1))
) ENGINE=InnoDB;
INSERT INTO authorization_runtime_schema VALUES(1,1,'303322599081b800a978b7a7b2b7c25f4e4f102fff518e73639d07b42609140a',1,TRUE);
CREATE TABLE authorization_actions (
 code VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 CHECK(code IN ('inventory.read','inventory.stock.write','order.amount.read','order.fulfillment.read','order.payment.read','order.read','price.read','product.read','product.write'))
) ENGINE=InnoDB;
INSERT INTO authorization_actions(code) VALUES ('inventory.read'),('inventory.stock.write'),('order.amount.read'),('order.fulfillment.read'),('order.payment.read'),('order.read'),('price.read'),('product.read'),('product.write');
CREATE TABLE authorization_principals (
 user_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 kind VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(kind IN ('employee','warehouse','supplier')),
 FOREIGN KEY(user_id) REFERENCES admin_users(id)
) ENGINE=InnoDB;
CREATE TABLE authorization_roles (
 id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 name VARCHAR(160) NOT NULL,
 enabled BOOLEAN NOT NULL CHECK(enabled IN (0,1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE authorization_role_actions (
 role_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 action_code VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 PRIMARY KEY(role_id,action_code),
 FOREIGN KEY(role_id) REFERENCES authorization_roles(id),
 FOREIGN KEY(action_code) REFERENCES authorization_actions(code)
) ENGINE=InnoDB;
CREATE TABLE authorization_memberships (
 user_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 role_id VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 PRIMARY KEY(user_id,role_id),
 FOREIGN KEY(user_id) REFERENCES authorization_principals(user_id),
 FOREIGN KEY(role_id) REFERENCES authorization_roles(id)
) ENGINE=InnoDB;
