# Project instructions

## Preserve existing functionality and styling

- Before adding or extending a feature, inspect the existing implementation and reuse or extend it where practical.
- When a feature already exists, preserve its existing behavior, interactions, and styling as much as possible. Make the smallest changes needed for the requested addition, and avoid unrelated rewrites, removals, or visual redesigns.
- If the requested addition requires changing existing behavior or styling, keep the change limited to what is necessary and explain its impact. Use the frontend theme palette below for any color changes.
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

- Use the theme palette in this file directly for the storefront, admin pages, standalone mobile pages, and shared components. Theme colors and their Ant Design state variants do not require additional user confirmation.
- A high-contrast color scheme and numerical contrast thresholds are not required. Keep the interface readable and consistent with the theme.
- Ensure text, controls, and important states remain clearly distinguishable against their backgrounds.

## UI theme and style — Ant Design reference

- Use [Ant Design](https://ant.design/docs/react/introduce-cn/) as the design-system reference, especially its [color specification](https://ant.design/docs/spec/colors-cn/). Structure colors into brand, functional, and neutral roles. The project-specific theme rules above take precedence over reference defaults.
- This section defines the design direction for the storefront, custom Admin pages, standalone mobile pages, and shared components. It does not authorize a site-wide restyle. Preserve existing interactions and migrate only the UI within the requested scope.
- Follow a clear, restrained, orderly style: establish hierarchy through typography, spacing, alignment, and neutral surfaces; reserve strong colors for actions and meaningful feedback. Avoid decorative gradients, heavy shadows, excessive rounding, and wrapping every text block in a card.

### Color roles and reference palette

Use the palette below as the project theme without requesting further color confirmation (updated by the user on 2026-10-23). Use Ant Design palette variants for related foreground, background, border, icon, hover, and pressed states as needed. Light gray `#F8F8F8` is the default gray surface. This theme applies across the storefront, custom Admin pages, standalone mobile pages, and shared components; it does not request a site-wide restyle.

| Role | Reference value | Intended use |
| --- | --- | --- |
| Brand | `#1677FF` | Primary actions, selected states, and important interaction cues |
| Strong brand | `#0958D9` | Darker action/link color; define hover and pressed states consistently before implementation |
| Brand surface | `#E6F4FF` | Subtle selection or informational background, paired with readable dark text and an explicit selection indicator |
| Light gray surface | `#F8F8F8` | Default gray page or card background; distinguish regions from adjacent white surfaces |
| White surface | `#FFFFFF` | White page or container background; pair with light gray surfaces to separate cards and regions |
| Primary text | `#000000E0` | Titles and primary content; alpha color, evaluated against the actual surface |
| Secondary text | `#000000A6` | Supporting descriptions and metadata; alpha color, evaluated against the actual surface |
| Default border | `#D9D9D9` | Reference border for containers; strengthen meaningful control boundaries when needed |
| Divider | `#0505050F` | Decorative separation between groups; alpha color, evaluated against the actual surface |
| Danger / error | `#F5222D` | Failed operations, validation errors, and destructive actions; use the same semantic base for danger and error |
| Success | `#52C41A` | Successful operations and completed states |
| Warning | `#FA8C16` | Risks, cautions, and states requiring attention |
| Rating stars | `#FADB14` | Ant Design Rate's default yellow; approved on 2026-10-22 specifically for storefront review displays and rating inputs, not as a replacement for warning or brand colors |

- Functional colors must consistently distinguish success, warning, error, and information. Use the functional base colors above and the brand palette for information. Select related state variants from the Ant Design palettes without additional confirmation. Never reuse an error color for ordinary emphasis.
- Status colors are semantic base colors. Keep readable neutral labels with status accents where appropriate; use Ant Design variants for status text and surfaces when needed.
- Use shared semantic tokens for text, surfaces, borders, dividers, actions, focus, and status. Reuse the project's existing token layer and Medusa semantic classes where available; avoid scattering literal color values through components.
- Do not combine legacy palette examples, current palette exports, and unrelated page-local colors into one theme. Keep theme choices consistent with the palette and semantic roles in this file.

### Surfaces, borders, and separation

- Minimize borders throughout the UI. Borders are allowed when they clarify structure or interaction, but should not be the default way to enclose cards, sections, or every row.
- Prefer background contrast to distinguish card layouts: white cards on a `#F8F8F8` canvas, or `#F8F8F8` cards on a white canvas. Use spacing, headings, and alignment for grouping. Avoid adding a border or shadow when the surfaces and spacing already communicate the grouping.
- Use subtle dividers selectively between table rows, list items, or sections only when they improve scanning. Avoid repeated separators and nested bordered containers. Do not use a pale divider or the subtle white/gray surface difference as the only indicator of an input, checkbox, selected option, or keyboard focus.
- Use distinct semantic tokens for container borders, control borders, and dividers. A decorative divider does not need the same prominence as an interactive boundary.
- Give links a non-color cue where needed, and communicate status with text and/or SVG icons as well as color.
- Keep focus clearly visible. Define hover, pressed, selected, loading, disabled, and error states together. Disabled controls must remain identifiable, with understandable labels and any necessary explanation.

### Typography, layout, and components

- Retain the established Outfit heading and DM Sans body fonts in the storefront, with system Chinese fallbacks. Keep Admin typography consistent with Medusa components. Use semantic headings and readable body line heights; avoid oversized marketing headings in operational screens.
- Use a consistent spacing scale based on 4px and 8px, emphasizing 8, 16, 24, and 32px. Retain the storefront's existing 1200px content-width convention. Let Admin tables and operational screens use the space needed for their data.
- Use restrained, consistent component radii and light elevation only where needed to communicate layering. Reuse existing shared component dimensions rather than introducing a new radius or shadow for each page.
- Give each section a clear primary action and distinguish secondary and destructive actions. Keep forms aligned, validation near the relevant fields, and tables readable with clear headers and row grouping. Preserve the existing Admin notification conventions.
- Storefront pages should prioritize authentic product imagery, model/configuration details, prices, stock, and the existing purchase or sell flow. Admin pages should prioritize scanning, filtering, comparison, and efficient operations.
- Reuse existing SVG icons and component libraries. Referencing Ant Design does not require installing `antd`, replacing Medusa UI, changing the font stack, or rewriting working components.
- For complex components such as data tables, advanced filters, tree selectors, multi-step forms, dialogs, and drawers, consult the [Ant Design component reference](https://ant.design/components/overview-cn/) for structure, interaction, and state handling. Adapt those patterns to the existing project components and the simple, restrained style above; preserve established business behavior.
- Support responsive layouts, keyboard navigation, visible focus, reduced motion, and both Simplified Chinese and English. Check the affected scope for readability, overflow, state clarity, and preserved interactions before completing a UI change.
