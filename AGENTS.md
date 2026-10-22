# Project instructions

## Preserve existing functionality and styling

- Before adding or extending a feature, inspect the existing implementation and reuse or extend it where practical.
- When a feature already exists, preserve its existing behavior, interactions, and styling as much as possible. Make the smallest changes needed for the requested addition, and avoid unrelated rewrites, removals, or visual redesigns.
- If the requested addition requires changing existing behavior or styling, keep the change limited to what is necessary and explain its impact. Follow the frontend color confirmation requirements below for any color changes.
- Verify that the added functionality works and that the affected existing functionality and styling remain intact.

## Local Admin debugging

- The user provides a dedicated debugging account and authorizes signing in to the local Admin at `http://localhost:9001/app` for development verification. Its `EMAIL` and `PASSWORD` are stored in the repository root `.admin-credentials`, which Git ignores. Read that file when a login is required; never print credentials in logs or commit them.
- Use the provided local account for read-only UI checks. Do not change real orders, devices, stock, or payments merely to test a UI change.

## Development service availability

- Keep the existing development services available while editing. Use `start.bat` on Windows or `sh/start.sh` on macOS/Linux; app `pnpm run dev` scripts use `scripts/dev-service.cjs` for supervision. Do not start a second raw `medusa develop` or `next dev` instance on ports 9001/8000.
- Before a batch of related backend edits, run `node scripts/dev-service.cjs backend pause` from this directory. The running backend continues serving its loaded code. In a `finally`/cleanup step after writing the complete batch, always run `node scripts/dev-service.cjs backend resume`, including when checks fail. The pause expires after 30 minutes as crash recovery; renew it for longer edits. Pause affects backend reloads/retries, not Admin/Next.js frontend HMR.
- Backend changes are coalesced after two seconds of quiet. API/workflow changes use the installed Medusa HMR protocol; shared libraries, modules and configuration trigger one full restart. HMR failure falls back to a restart. Test files, standalone scripts and migrations do not trigger restarts; apply migrations explicitly before resuming when required.
- Do not kill all Node processes, change ports, delete build output or repeatedly restart services to make a check pass. Use `node scripts/dev-service.cjs backend restart` for a necessary controlled restart. To intentionally stop, use `node scripts/dev-service.cjs backend stop` and/or `node scripts/dev-service.cjs storefront stop`, so the supervisor does not interpret shutdown as a crash.
- Never run `next build`/`medusa build` or clear `.next`/`.medusa` in the same checkout while its dev service is running. Validate with lint/type checks first; use a separate checkout with its own dependencies for a required build, or deliberately stop the affected service, build, then restore it. Do not launch a second backend against the same database/Redis as an improvised stable preview; workers/jobs can execute twice. A stable preview needs isolated configuration and data.
- Finish development tasks by verifying `http://127.0.0.1:9001/health`, `http://127.0.0.1:9001/app` and the affected storefront route, and inspect recent service logs. Report any unresolved outage rather than leaving the user to discover it.

## Frontend colors

- All frontend pages must use a high-contrast color scheme. This applies to the storefront, admin pages, standalone mobile pages, and their shared components.
- Before using or changing any interface colors, present the proposed colors and their intended uses to the user and obtain explicit confirmation. Do not implement unconfirmed colors or treat silence as approval.
- Existing user confirmation applies only within its stated scope. Do not extend approval for one page's colors to other pages without confirmation.
- Ensure text, controls, and important states remain clearly distinguishable against their backgrounds.
