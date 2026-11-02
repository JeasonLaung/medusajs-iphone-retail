-- Product creation is independent from existing basic edits. No automatic enable.
INSERT INTO catalog_command_capabilities(command,version,enabled) VALUES('product.create',1,FALSE);
