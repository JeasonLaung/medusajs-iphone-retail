# SKU numbering

When a new variant's SKU is omitted, null, empty or whitespace, PostgreSQL assigns
`ERP` + `(numeric ID + 10000)`: ID 1 is `ERP10001`, ID 2 is `ERP10002`.
The numeric ID is a dedicated global sequence, separate from Medusa's string
variant ID. `P` stands for phone. The number is not truncated or padded.

The allocator row serializes concurrent writers across processes and restarts.
Committed allocations survive variant deletion; failed database transactions
roll back their allocation. Explicit nonempty SKUs are preserved. Explicit ERP
numbers advance the allocator to avoid future collisions. Clearing an existing
generated SKU retains that SKU. The trigger covers admin, API, import and workflow
creation paths. Generated numbers remain stable when options change.

The variant-created workflow hook fills empty default inventory SKUs from the
persisted generated code before creation completes. Custom and shared inventory
codes are preserved. Standard admin endpoints refetch the generated SKU; direct
workflow callers should also read the persisted variant instead of an input SKU.

Run `pnpm exec medusa db:migrate` from the backend to install the trigger.
This module has no DML models: its handwritten migration owns the counter,
function and trigger. Do not generate a schema migration that drops the counter.

Existing iPhone variants are migrated separately through Medusa workflows:

```powershell
pnpm exec medusa exec ./src/scripts/normalize-iphone-skus.ts
$env:NORMALIZE_IPHONE_SKUS = "apply"
pnpm exec medusa exec ./src/scripts/normalize-iphone-skus.ts
Remove-Item Env:NORMALIZE_IPHONE_SKUS
```

The script backs up previous SKUs, retains source codes in `metadata.source_sku`,
updates linked inventory SKUs, and checks prices, inventory and variant IDs.
Re-running it preserves already assigned numbers. Configuration slugs remain in
`metadata.slug` for matching imported options independently of the SKU.

Run `node --test tests/sku-numbering.test.cjs` to verify allocation, concurrent
writers and rollback against the local Docker Compose database. Concurrency
tests use an isolated schema and do not consume real SKU numbers.
Run `pnpm exec medusa exec ./src/scripts/check-sku-numbering.ts` for the full
product/variant workflow check. It cleans up its draft product afterward; its
generated numbers remain reserved, like any other deleted variant.
