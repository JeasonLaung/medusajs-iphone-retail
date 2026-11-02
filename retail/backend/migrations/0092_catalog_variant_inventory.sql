INSERT INTO catalog_command_capabilities VALUES('variant.inventory',1,FALSE);
CREATE TABLE catalog_inventory_edits LIKE catalog_image_edits;
ALTER TABLE catalog_inventory_edits
 ADD FOREIGN KEY(product_id) REFERENCES catalog_products(id),
 ADD FOREIGN KEY(actor_id) REFERENCES admin_users(id);
