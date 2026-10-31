-- Supplemental cash and the explicitly owned new-account inbox commit together.
-- Preserve the existing event identity and all prior notification kinds.
ALTER TABLE customer_account_notices
 DROP CHECK customer_account_notices_kind,
 ADD CONSTRAINT customer_account_notices_kind CHECK(kind IN('shipment.created','delivery.created','order.canceled','after_sales.processing','after_sales.completed','after_sales.rejected','buyback.updated','refund.created','payment.received'));
