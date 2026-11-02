INSERT INTO catalog_command_capabilities VALUES('product.options',1,FALSE);
CREATE TABLE catalog_option_edits LIKE catalog_image_edits;
ALTER TABLE catalog_option_edits
 ADD FOREIGN KEY(product_id) REFERENCES catalog_products(id),
 ADD FOREIGN KEY(actor_id) REFERENCES admin_users(id);
