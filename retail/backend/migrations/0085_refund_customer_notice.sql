-- A refund notice is committed with its unique local cash movement. Existing
-- notification identities, recipient ownership and event uniqueness remain.
ALTER TABLE customer_account_notices
 DROP CHECK customer_account_notices_chk_1,
 ADD CONSTRAINT customer_account_notices_kind CHECK(kind IN('shipment.created','delivery.created','order.canceled','after_sales.processing','after_sales.completed','after_sales.rejected','buyback.updated','refund.created'));
