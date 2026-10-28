# Local visual preview

Run `node preview.mjs` from `retail/storefront` using Node >=22.23.4 and open http://127.0.0.1:9300/dk-zh/preview. The runner binds loopback only and uses a separate Astro root.

This assembles the migrated product gallery, configuration controls, grade guide, details, cart and footer with explicit sample data. The original header/menu and region drawer are included. The extra preview controls/banner are a demo shell. Region/language changes navigate within this sample, resetting in-memory cart state; production cart region/locale mutations remain pending. Cart commands update in-memory sample state and reset on reload. Checkout is disabled. No production page, Gin transaction or full storefront acceptance is claimed.

The original global CSS, migrated components and copied original images are reused. The preview does not start Gin, MySQL, Redis or the existing development services. It never changes real stock, devices, orders or payments.
