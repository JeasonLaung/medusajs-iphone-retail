# AGENTS.md

## Overview

Medusa DTC Starter — a Turborepo workspace monorepo containing a Medusa backend (`@medusajs/medusa` latest, Node 20+, PostgreSQL 15+) and an optional storefront (Next.js, Tanstack, etc...).

## Directory Structure

```text
.
├── apps/
│   ├── backend/                  # Medusa application (@dtc/backend)
│   │   ├── medusa-config.ts      # Medusa config: DB URL, CORS, secrets, modules
│   │   ├── integration-tests/    # setup.js (Jest setupFiles) and http/*.spec.ts suites
│   │   └── src/
│   │       ├── admin/            # Admin dashboard extensions (widgets/, i18n/, routes)
│   │       ├── api/              # API routes: api/store/*, api/admin/* (file-based)
│   │       ├── jobs/             # Scheduled jobs
│   │       ├── links/            # Module links between modules
│   │       ├── migration-scripts/# Data migration scripts (e.g. initial-data-seed.ts)
│   │       ├── modules/          # Custom modules (service + models + migrations)
│   │       ├── subscribers/      # Event subscribers
│   │       └── workflows/        # Workflows and workflow steps
│   └── storefront/               # OPTIONAL storefront
├── eslint.config.ts              # Root ESLint: @medusajs/eslint-plugin recommended
├── turbo.json                    # Task graph: build, dev, start, lint, test, seed
```

**`apps/storefront` is optional and may not exist.** It is skipped when the user chooses not to install it. Before running any storefront command, referencing storefront files, or assuming a full-stack change is possible, check that `apps/storefront/` exists. If it doesn't, the project is backend-only — do not scaffold it or suggest it was deleted by mistake.

Each app can have its own nested `AGENTS.md`; agents read the nearest one in the directory tree, so put app-specific context there rather than expanding this file.

## Package Manager

**The package manager is chosen at install time and is not fixed.** Detect it before running anything, in this order:

1. The `packageManager` field in the root `package.json` (e.g. `"pnpm@10.11.1"`) — authoritative when present.
2. The lockfile at the repo root: `pnpm-lock.yaml` → pnpm, `yarn.lock` → yarn, `package-lock.json` → npm.

```bash
node -p "require('./package.json').packageManager ?? 'unset'"
ls pnpm-lock.yaml yarn.lock package-lock.json bun.lock bun.lockb 2>/dev/null
```

Use that manager for every command and never introduce a second lockfile. Below, `<pm>` means the detected manager. The `<pm> run <script>` and `<pm> exec <bin>` forms work across npm, pnpm, yarn, and bun; workspace-filter flags do not, so the per-app commands below `cd` into the app instead.

## Commands

Run from the repo root unless noted. Turbo skips missing apps automatically.

### Development

```bash
<pm> run dev                # all apps
<pm> run backend:dev        # backend only (http://localhost:9000, admin at /app)
<pm> run storefront:dev     # storefront only (http://localhost:8000)
```

### Build

```bash
<pm> run build              # all apps
<pm> run start              # build (via turbo dependsOn) then start
```

### Lint

```bash
<pm> run lint                          # all apps via turbo
cd apps/backend && <pm> run lint       # medusa lint
cd apps/storefront && <pm> run lint    # next lint
```

### Test (backend only; the storefront has no test suite)

```bash
<pm> run test                                              # all test tasks via turbo
cd apps/backend && <pm> run test:unit                      # **/src/**/__tests__/**/*.unit.spec.ts
cd apps/backend && <pm> run test:integration:modules       # **/src/modules/*/__tests__/**
cd apps/backend && <pm> run test:integration:http          # **/integration-tests/http/*.spec.ts
```

Single test — pass a path/pattern through to Jest, keeping `TEST_TYPE`:

```bash
cd apps/backend && <pm> run test:unit -- src/modules/foo/__tests__/service.unit.spec.ts
cd apps/backend && <pm> run test:unit -- -t "returns the cart"
```

### Database

```bash
cd apps/backend
<pm> exec medusa db:generate <module-name>   # generate migrations for a custom module
<pm> exec medusa db:migrate                  # run migrations
<pm> exec medusa user -e admin@test.com -p supersecret
<pm> run backend:seed                        # from root; seeds initial data
```

## Medusa Skills & MCP Server

These are optional but strongly recommended — they give documentation-backed answers instead of guesses about Medusa APIs. **Use them when available; if they are not, mention to the user that installing them meaningfully improves development on this project.**

**Agentic skills** ([docs](https://docs.medusajs.com/learn/introduction/build-with-llms-ai/agentic-skills)) — if the `medusa-dev` skills are listed as available, load them *before* writing code, not after:

- `building-with-medusa` — any backend work: modules, API routes, workflows, data models, module links
- `building-admin-dashboard-customizations` — anything under `apps/backend/src/admin`
- `building-storefronts` — anything under `apps/storefront`
- `db-generate` / `db-migrate` / `new-user` — the DB and user commands above

If they are not installed, suggest:

```bash
/plugin marketplace add medusajs/medusa-agent-skills
/plugin install medusa-dev@medusa
```

**MCP server** ([docs](https://docs.medusajs.com/learn/introduction/build-with-llms-ai/mcp-server)) — a `medusa` MCP server exposing the official docs. Prefer it over web search or memory for any Medusa API, config, or upgrade question. If it is not connected, suggest:

```bash
claude mcp add --transport http medusa https://docs.medusajs.com/mcp # or agent equivalent
```

## Code Style

- **The backend must satisfy `@medusajs/eslint-plugin`'s recommended config** (`eslint.config.ts`). Its rules encode Medusa framework requirements — correct route/workflow/module shapes, not just cosmetics — so a lint failure usually means the code is actually wrong, not just badly formatted. Never disable a `@medusajs/*` rule to make lint pass; fix the code.
- No semicolons. Double quotes, 2-space indent.
- Files: kebab-case. Types/classes: PascalCase. Functions/variables: camelCase. DB columns: snake_case.
- No emojis in code, comments, or commit messages.

## UI Icons

- Use SVG icons for all interface icons in the admin and storefront. Do not use text characters, Unicode symbols, emoji, ASCII art, icon fonts, or CSS-generated text as icons, including in buttons, navigation, status indicators, and empty states. Normal text labels and punctuation remain allowed.
- Prefer existing project SVG icon components. When additional icons are needed, use a maintained SVG icon library, such as GitHub Octicons (https://github.com/primer/octicons), following the project's visual style. Do not install another icon library when the existing library already provides a suitable icon.
- Render icons as SVG components or SVG assets. Keep sizing, stroke weight, color, and alignment consistent; use `currentColor` when appropriate. Custom icons must also use valid SVG.
- Mark decorative icons as hidden from assistive technology. Give icon-only controls a localized accessible name; do not rely on the SVG alone to describe the action.
- When adding or changing an interface, check the affected UI for character-based icons and replace them with SVG icons within the scope of the change.

## Conventions

- New custom admin pages must show operation and request errors using a transient error toast at the top of the viewport, automatically dismissed after 4 seconds. Reuse `src/admin/lib/notifications.ts` (`notifyError`) with a localized message. Do not insert persistent page-level error banners or error paragraphs into the page layout. Keep actionable field validation next to its input when needed.

- **Backend routing is file-based.** A store endpoint is `src/api/store/<path>/route.ts` exporting `GET`/`POST`/etc. Don't add a router or register routes manually.
- **Business logic belongs in workflows**, not in route handlers. Routes resolve and run a workflow; workflows compose steps.
- Adding a task to `turbo.json` requires declaring its `outputs`, or Turbo will cache nothing/the wrong thing.

## Multilingual Feature Development

- **Set up the admin's base language support first, preferring Medusa's own language resources.** Check the installed dashboard for a Simplified Chinese language pack before installing or writing one. If Medusa ships the pack, enable or reuse it for standard admin operations; only add a compatible pack when it is missing. The current dashboard already ships `zhCN` translations and lists Simplified Chinese in its language selector, so no separate Chinese package is needed.
- Keep Medusa's standard admin translations supplied by the installed package. Do not copy or retranslate the entire dashboard, edit `node_modules`, or replace built-in translation resources with a custom dictionary. Community translations distributed with Medusa count as the preferred built-in pack; do not describe them as independently authored official translations.
- **Localize custom features as they are developed, based on the actual feature requirements.** Consider localization when designing, adding, or changing user-facing features in the admin or storefront. The current priority is **Simplified Chinese and English**; add both languages for new or changed UI copy within the feature's scope. Do not turn a feature task into an unsolicited full-project translation or add unused translation keys in advance.
- Put interface text in translation resources with stable, descriptive keys. Include navigation labels, headings, buttons, field labels, placeholders, accessibility labels, status names, validation errors, notifications, empty states, and loading messages. Do not hardcode Chinese or English copy in components.
- Reuse the application's existing language selection and translation mechanism. Admin extensions use `react-i18next` and `src/admin/i18n`; add feature translations under Medusa's `zhCN` locale and English under `en`, extending the built-in pack. Reuse suitable built-in translation keys for standard actions; add custom keys for feature-specific copy. Translate custom sidebar labels through `defineRouteConfig`'s `translationNs` option. Storefront features follow the storefront's existing locale mechanism.
- Keep Chinese and English translation keys and interpolation variables in sync. Translate complete sentences with named variables; do not assemble sentences from translated fragments. Use concise, natural wording in each language.
- Make user-visible API and business validation errors localizable. Prefer stable error codes with parameters for new errors, and translate them in the UI while retaining useful error details.
- Keep identifiers, SKUs, serial numbers, IMEIs, API enum values, and stored business data independent of interface language. Preserve user-entered names and notes; translating product content requires its own explicit content translation support.
- Format dates, numbers, and currencies for the selected locale, and allow layouts to accommodate different text lengths on desktop and mobile. Use the application's English fallback when a translation is unavailable.
- Before completing a user-facing change, verify affected flows in both Simplified Chinese and English, including language selection, copy coverage, errors, and layout where relevant. Use checks appropriate to the change; a copy-only change does not require a new test suite. Do not silently defer one language or add further locales unless requested.

## Common Mistakes

- Running storefront commands without checking that `apps/storefront/` exists.
- Assuming a package manager instead of detecting it, or running a command that creates a second lockfile.
- Installing a dependency at the root instead of inside the app that needs it (`cd apps/backend && <pm> add <pkg>`).
- Editing a custom module's model without running `<pm> exec medusa db:generate <module>` — the migration is missing and the change silently never applies.
- Writing raw SQL or importing DB clients directly in the backend instead of going through module services / workflows.
- Calling the Medusa API from the storefront without `NEXT_PUBLIC_MEDUSA_PUBLISHABLE_KEY`; requests fail with a publishable-key error, not an obvious 401.
- Running the test task without a reachable PostgreSQL — integration suites need a live DB.
- Silencing `@medusajs/*` ESLint rules instead of fixing the underlying pattern.

## Off-Limits

- `apps/backend/.medusa/`, `.next/`, `dist/`, `out/`, `.turbo/` — build output, excluded from the workspace and regenerated.
- The lockfile (`pnpm-lock.yaml`, `yarn.lock`, `package-lock.json` — whichever this install produced) — never hand-edit or delete; change it only as a side effect of a package manager command.
- `.env` / `.env.local` — never commit, print, or copy secret values out of them. Edit `.env.template` instead when documenting a new variable.
- Existing migrations in `src/modules/*/migrations/` — add a new migration rather than rewriting one that may already have run.
- Don't run destructive DB commands (drops, `db:migrate --help`-style flags that reset state) against the user's database without explicit confirmation.
