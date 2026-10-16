# Order numbering

New orders receive a persisted `custom_display_id` in the format
`YYYYMMDDHHmmssSSSSS` (19 characters), using `Asia/Shanghai` time.
The five trailing digits start from fractional seconds at 10-microsecond
resolution. A database clock advances on collisions, clock rollback and suffix
overflow, so this is a monotonic identifier rather than an exact timestamp.

A PostgreSQL BEFORE INSERT trigger allocates the number within the order
transaction. Its singleton clock row serializes writers across processes and
restarts. Medusa's existing unique index on `order.custom_display_id` provides
an additional constraint. All order creation paths use the same allocator,
including admin and draft orders. Allocation rolls back with failed inserts.
Historical orders are not renumbered. Storefronts display `custom_display_id`
with a fallback to the original `display_id`.

Run `corepack pnpm exec medusa db:migrate` before using a new environment.
The module has no DML models: its handwritten migrations own its clock table,
function and trigger. Do not generate a schema migration that drops that table.

Verification: run `node --test tests/order-numbering.test.cjs` from the backend.
It uses the local Docker Compose PostgreSQL container and rolls back all test
orders. Run `node --test tests/order-number.test.cjs` from the storefront for
historical order and string precision checks.
