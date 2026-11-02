-- Keep the original CREATE input/cart version immutable. Context changes that
-- retain the exact payment amount/session bind a separately current revision.
-- NULL preserves the original binding for every existing checkout.
ALTER TABLE cart_checkout_attempts
 ADD COLUMN effective_cart_version BIGINT UNSIGNED NULL,
 ADD CONSTRAINT ck_checkout_effective_version
 CHECK(effective_cart_version IS NULL OR effective_cart_version >= cart_version);
