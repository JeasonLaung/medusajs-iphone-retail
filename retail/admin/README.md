# React Admin

Independent Gin client. No Medusa SDK or runtime. Current implemented routes:
`/login` and `/account` only. Business pages and granular authorization are
still pending; the account menu is not a completed merchant dashboard.

Use Node 22.22+ and the committed npm lockfile:

```powershell
npm ci
npm run dev
```

Open `http://127.0.0.1:9200`. Development proxy preserves Host and Origin and
forwards `/api` to the separate Gin process at `127.0.0.1:9100`. Gin must use
`ADMIN_ORIGIN=http://127.0.0.1:9200`, `SESSION_SECURE=false`, explicit dedicated
MySQL/Redis configuration and explicit schema migrations. Do not point the new
API at a legacy database or reuse another application's Redis namespace.
No services or accounts are created by the frontend. Production requires the
same-origin API proxy, explicit HTTPS Origin and Secure cookies.

Check with `npm run check` and `npm run build`. Build output is ignored and is
independent of the existing Medusa/Next development services.

`npm test` starts an owned Vite test server on `127.0.0.1:19200` with strict port
selection, runs the artificial HTTP browser cases, and closes that server.
Never reuse or stop a process already owning that port. Playwright requires an
installed browser; an existing local browser can be selected explicitly:

```powershell
$env:ADMIN_TEST_BROWSER='C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
npm test
```

Only artificial identities appear in these tests. Screenshots and failure
artifacts go to the repository's ignored `logs/gin-migration/admin-ui` folder.
The reproducible report wrapper is `python retail/scripts/test-admin-ui.py`
from the repository root. These UI fixtures alone do not prove a real browser
login against Gin/MySQL/Redis.

For that separate live check, run `python retail/scripts/test-admin-live.py`
from the repository root with the same optional browser override. It builds the
actual Gin executable and uses owned disposable MySQL/Redis containers, nine
schema migrations and one artificial new account. No intercepted API responses
are used. It requires the existing `mysql:8.4` and `redis:7-alpine` images and
never pulls images. Only a randomly allocated API port is published on host
loopback; SQL/Redis have no published ports. The dedicated bridge does not
restrict outbound traffic. Owned containers, network and binaries are removed.
`ADMIN_TEST_API_TARGET` is an explicit loopback-only Vite proxy override used
by this runner; normal development still defaults to port 9100.

The live check covers loopback HTTP cookies, CSRF, Origin, reload, logout and
old-session replay. It does not establish production HTTPS/Secure-cookie or
customer, permission and business-module acceptance.

Ant Design 5 is pinned to the inspected reference generation. React 19 uses
Ant Design's official compatibility patch. There are no charts yet; all future
Admin charts must use Apache ECharts. Do not add fabricated statistics.
