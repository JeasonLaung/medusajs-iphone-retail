INSERT INTO catalog_command_capabilities VALUES('variant.edit',1,FALSE);
CREATE TABLE catalog_variant_edits LIKE catalog_image_edits;
ALTER TABLE catalog_variant_edits
 ADD FOREIGN KEY(product_id) REFERENCES catalog_products(id),
 ADD FOREIGN KEY(actor_id) REFERENCES admin_users(id);
