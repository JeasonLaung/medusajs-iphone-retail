-- iPhone inventory synchronization performed on 2026-10-21.
-- The live update used Medusa workflows so inventory events, reservations,
-- search indexing, and generated IDs remain consistent. This SQL records the
-- equivalent data changes for audit/review; run only against a backup copy.

BEGIN;

-- Prefix every iPhone inventory item's title with its model. The join table is
-- used because inventory_item.title is the title shown by inventory screens.
WITH iphone_inventory AS (
  SELECT DISTINCT
    pvi.inventory_item_id,
    CASE
      WHEN pv.title LIKE p.title || ' / %' THEN pv.title
      WHEN NULLIF(BTRIM(pv.title), '') IS NULL THEN p.title
      ELSE p.title || ' / ' || pv.title
    END AS desired_title
  FROM product_variant_inventory_item pvi
  JOIN product_variant pv ON pv.id = pvi.variant_id
  JOIN product p ON p.id = pv.product_id
  WHERE p.handle LIKE 'swappie-%'
)
UPDATE inventory_item ii
SET title = iphone_inventory.desired_title,
    updated_at = NOW()
FROM iphone_inventory
WHERE ii.id = iphone_inventory.inventory_item_id
  AND ii.title IS DISTINCT FROM iphone_inventory.desired_title;

-- Add 50 units to existing levels for iPhone inventory in the default-sales-
-- channel warehouse. Missing levels were created by the live workflow with 50.
WITH default_warehouse AS (
  SELECT sl.id
  FROM stock_location sl
  JOIN stock_location_sales_channel slsc ON slsc.stock_location_id = sl.id
  JOIN store s ON s.default_sales_channel_id = slsc.sales_channel_id
), iphone_inventory AS (
  SELECT DISTINCT pvi.inventory_item_id
  FROM product_variant_inventory_item pvi
  JOIN product_variant pv ON pv.id = pvi.variant_id
  JOIN product p ON p.id = pv.product_id
  WHERE p.handle LIKE 'swappie-%'
)
UPDATE inventory_level il
SET stocked_quantity = il.stocked_quantity + 50,
    updated_at = NOW()
FROM default_warehouse dw, iphone_inventory ii
WHERE il.location_id = dw.id
  AND il.inventory_item_id = ii.inventory_item_id;

COMMIT;
