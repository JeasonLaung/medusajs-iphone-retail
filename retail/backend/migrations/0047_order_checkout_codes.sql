-- Cart tax/adjustment codes already accept 191 characters. New order snapshots
-- must preserve the full saved code, including Unicode, without SQL truncation.
ALTER TABLE order_line_adjustments MODIFY COLUMN code VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL;
ALTER TABLE order_line_tax_lines MODIFY COLUMN code VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL;
ALTER TABLE order_shipping_adjustments MODIFY COLUMN code VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL;
ALTER TABLE order_shipping_tax_lines MODIFY COLUMN code VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL;
-- Bind accepted paid state to its exact sealed external evidence. A mutable
-- status string alone must never authorize creation of a new order snapshot.
ALTER TABLE cart_checkout_attempts ADD COLUMN paid_observation_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL;
ALTER TABLE cart_checkout_attempts ADD CONSTRAINT fk_checkout_paid_observation FOREIGN KEY(paid_observation_id) REFERENCES cart_checkout_observations(id);
