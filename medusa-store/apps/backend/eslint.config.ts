import { defineConfig } from "eslint/config"
import medusa from "@medusajs/eslint-plugin"

export default defineConfig([
  { ignores: ["static/mobile-intake/vendor/**"] },
  ...medusa.configs.recommended,
])
