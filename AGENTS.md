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

## UI theme and style — Ant Design reference

- Use [Ant Design](https://ant.design/docs/react/introduce-cn/) as the design-system reference, especially its [color specification](https://ant.design/docs/spec/colors-cn/). Structure colors into brand, functional, and neutral roles. The project-specific accessibility and color-confirmation rules above take precedence over reference defaults.
- This section defines the design direction for the storefront, custom Admin pages, standalone mobile pages, and shared components. It does not authorize a site-wide restyle. Preserve existing interactions and migrate only the UI within the requested scope.
- Follow a clear, restrained, orderly style: establish hierarchy through typography, spacing, alignment, and neutral surfaces; reserve strong colors for actions and meaningful feedback. Avoid decorative gradients, heavy shadows, excessive rounding, and wrapping every text block in a card.

### Color roles and reference palette

The brand and remaining neutral values below remain reference proposals based on Ant Design. The user explicitly confirmed the functional base colors on 2026-10-22: danger/error `#F5222D`, success `#52C41A`, and warning `#FA8C16`. The user also confirmed white `#FFFFFF` and light gray `#F8F8F8` for background-based card and layout separation, with `#F8F8F8` as the default gray surface. These confirmations define the relevant colors for this project's UI theme; they do not request a site-wide restyle or approve additional derived colors. Present unconfirmed colors, foreground/background pairings, and any expanded page scope for explicit confirmation before applying them. Do not treat the user's choice of Ant Design as blanket approval of every color or every page.

| Role | Reference value | Intended use |
| --- | --- | --- |
| Brand | `#1677FF` | Primary actions, selected states, and important interaction cues |
| Strong brand | `#0958D9` | Candidate darker action/link color where contrast requires it; define hover and pressed states consistently before implementation |
| Brand surface | `#E6F4FF` | Subtle selection or informational background, paired with readable dark text and an explicit selection indicator |
| Light gray surface (confirmed) | `#F8F8F8` | Default gray page or card background; distinguish regions from adjacent white surfaces |
| White surface (confirmed) | `#FFFFFF` | White page or container background; pair with light gray surfaces to separate cards and regions |
| Primary text | `#000000E0` | Titles and primary content; alpha color, evaluated against the actual surface |
| Secondary text | `#000000A6` | Supporting descriptions and metadata; alpha color, evaluated against the actual surface |
| Default border | `#D9D9D9` | Reference border for containers; strengthen meaningful control boundaries when needed |
| Divider | `#0505050F` | Decorative separation between groups; alpha color, evaluated against the actual surface |
| Danger / error (confirmed) | `#F5222D` | Failed operations, validation errors, and destructive actions; use the same semantic base for danger and error |
| Success (confirmed) | `#52C41A` | Successful operations and completed states |
| Warning (confirmed) | `#FA8C16` | Risks, cautions, and states requiring attention |
| Rating stars (confirmed) | `#FADB14` | Ant Design Rate's default yellow; approved on 2026-10-22 specifically for storefront review displays and rating inputs, not as a replacement for warning or brand colors |

- Functional colors must consistently distinguish success, warning, error, and information. Use the confirmed functional base colors above; information remains an unconfirmed theme role. Select any additional foreground, background, border, icon, hover, and pressed variants from the reference palettes, verify contrast, and obtain confirmation before use. Never reuse an error color for ordinary emphasis.
- Confirmed status colors are semantic base colors, not automatically accessible text colors or button/text combinations. In particular, do not assume success green or warning orange works as small text on white or as a background with white text. Keep readable neutral labels with status accents where appropriate; any darker text variants or lighter status surfaces require confirmation before introduction.
- Use shared semantic tokens for text, surfaces, borders, dividers, actions, focus, and status. Reuse the project's existing token layer and Medusa semantic classes where available; avoid scattering literal color values through components.
- Do not combine legacy palette examples, current palette exports, and unrelated page-local colors into one theme. Record the chosen palette and its approved scope when the user confirms it.

### Surfaces, borders, and separation

- Minimize borders throughout the UI. Borders are allowed when they clarify structure or interaction, but should not be the default way to enclose cards, sections, or every row.
- Prefer background contrast to distinguish card layouts: white cards on a `#F8F8F8` canvas, or `#F8F8F8` cards on a white canvas. Use spacing, headings, and alignment for grouping. Avoid adding a border or shadow when the surfaces and spacing already communicate the grouping.
- Use subtle dividers selectively between table rows, list items, or sections only when they improve scanning. Avoid repeated separators and nested bordered containers. Do not use a pale divider or the subtle white/gray surface difference as the only indicator of an input, checkbox, selected option, or keyboard focus.
- Use distinct semantic tokens for container borders, control borders, and dividers. A decorative divider does not need the same prominence as an interactive boundary.
- Normal text must reach at least 4.5:1 contrast; large text and necessary visual indicators of controls/states must reach at least 3:1 against adjacent colors. Verify actual composited colors, including alpha values. Adjust reference colors when required and obtain confirmation for the resulting palette.
- Do not assume white text on the reference brand blue, light status text, pale borders, or placeholder text automatically meet these requirements. Give links a non-color cue where needed, and communicate status with text and/or SVG icons as well as color.
- Keep focus clearly visible. Define hover, pressed, selected, loading, disabled, and error states together. Disabled controls must remain identifiable, with understandable labels and any necessary explanation.

### Typography, layout, and components

- Retain the established Outfit heading and DM Sans body fonts in the storefront, with system Chinese fallbacks. Keep Admin typography consistent with Medusa components. Use semantic headings and readable body line heights; avoid oversized marketing headings in operational screens.
- Use a consistent spacing scale based on 4px and 8px, emphasizing 8, 16, 24, and 32px. Retain the storefront's existing 1200px content-width convention. Let Admin tables and operational screens use the space needed for their data.
- Use restrained, consistent component radii and light elevation only where needed to communicate layering. Reuse existing shared component dimensions rather than introducing a new radius or shadow for each page.
- Give each section a clear primary action and distinguish secondary and destructive actions. Keep forms aligned, validation near the relevant fields, and tables readable with clear headers and row grouping. Preserve the existing Admin notification conventions.
- Storefront pages should prioritize authentic product imagery, model/configuration details, prices, stock, and the existing purchase or sell flow. Admin pages should prioritize scanning, filtering, comparison, and efficient operations.
- Reuse existing SVG icons and component libraries. Referencing Ant Design does not require installing `antd`, replacing Medusa UI, changing the font stack, or rewriting working components.
- For complex components such as data tables, advanced filters, tree selectors, multi-step forms, dialogs, and drawers, consult the [Ant Design component reference](https://ant.design/components/overview-cn/) for structure, interaction, and state handling. Adapt those patterns to the existing project components and the simple, restrained style above; preserve established business behavior.
- Support responsive layouts, keyboard navigation, visible focus, reduced motion, and both Simplified Chinese and English. Check the affected scope for contrast, overflow, state clarity, and preserved interactions before completing a UI change.
