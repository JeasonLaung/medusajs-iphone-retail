# Migration application

- This application replaces the Medusa runtime with Gin, React Admin and Astro. Do not proxy to Medusa as an implementation shortcut.
- The existing `../medusa-store` is the business and storefront appearance reference. Keep its development services and data intact.
- Use a dedicated MySQL database. Never import into, migrate or change another project's database.
- Schema changes are explicit, versioned SQL in `backend/migrations`; application startup must not create or alter tables.
- Preserve source IDs, soft deletion, exact monetary values and protected file access. Customer and admin authentication are distinct.
- Mark incomplete business domains honestly in `MIGRATION.md`; a menu item or CRUD endpoint does not mean its workflow is migrated.
- Copy storefront markup, CSS, images and translations with provenance; do not redesign pages during migration.
- Reference `iphone0902` for admin layout and foundations only. Do not import its unrelated inquiry/crawler/deployment business.
