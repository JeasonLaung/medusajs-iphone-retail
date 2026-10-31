-- Version-2 workers cannot recheck a native exchange credit before dispatch.
-- Preserve the existing default-off setting; explicit operator enable remains required.
ALTER TABLE order_refund_runtime DROP CHECK ck_order_refund_attempt_runtime;
UPDATE order_refund_runtime SET version=3;
ALTER TABLE order_refund_runtime ADD CONSTRAINT ck_order_refund_exchange_runtime CHECK(version=3);
