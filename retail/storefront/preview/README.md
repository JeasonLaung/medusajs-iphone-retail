# Local visual preview

Run `node preview.mjs` from `retail/storefront` using Node >=22.23.4 and open http://127.0.0.1:9300/dk-zh/preview. The runner binds loopback only and uses a separate Astro root.

This assembles the migrated product gallery, configuration controls, grade guide, details, cart and footer with explicit sample data. The preview navigation/banner is a demo shell; the production header remains pending. Cart commands update in-memory sample state and reset on reload. Checkout is disabled. No production page, Gin transaction or full storefront acceptance is claimed.

The original global CSS, migrated components and copied original images are reused. The preview does not start Gin, MySQL, Redis or the existing development services. It never changes real stock, devices, orders or payments.
