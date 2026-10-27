# Migration application

- This application replaces the Medusa runtime with Gin, React Admin and Astro. Do not proxy to Medusa as an implementation shortcut.
- The existing `../medusa-store` is the business and storefront appearance reference. Keep its development services and data intact.
- Use dedicated MySQL and Redis resources. Never import into, migrate or change another project's database or Redis keys. MySQL owns durable business facts; Redis caches/sessions/rate limits must not be the sole storage for inventory, payments, durable idempotency or reliable jobs.
- Do not migrate legacy customer/staff login accounts, password hashes, sessions or authentication tokens (user decision, 2026-10-27). Create new accounts; preserve historical business snapshots and legacy actor references without granting new accounts ownership based on matching email.
- Schema changes are explicit, versioned SQL in `backend/migrations`; application startup must not create or alter tables.
- Preserve source IDs, soft deletion, exact monetary values and protected file access. Customer and admin authentication are distinct.
- Mark incomplete business domains honestly in `MIGRATION.md`; a menu item or CRUD endpoint does not mean its workflow is migrated.
- Copy storefront markup, CSS, images and translations with provenance; do not redesign pages during migration.
- Reference `iphone0902` for admin layout and foundations only. Do not import its unrelated inquiry/crawler/deployment business.
- New React Admin UI should use Ant Design components wherever practical (user instruction, reconfirmed 2026-10-28). Reuse Form, Table, Layout, Menu, Tabs, Drawer, Modal, Upload, feedback and other suitable components instead of rebuilding their primitives. Keep domain-specific business logic and existing workflow semantics.
- Use Apache ECharts for all new Admin charts. Dispose instances, observe container resizing and provide loading, empty and error states. This Admin choice does not authorize changing storefront appearance or its component library.
