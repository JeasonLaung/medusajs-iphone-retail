import { defineConfig } from '@playwright/test'
import base from './playwright.config'
import path from 'node:path'

if (!process.env.ADMIN_TEST_API_TARGET) throw new Error('Owned isolated live API required')
export default defineConfig({
  ...base,
  testMatch: ['auth.live.spec.ts', 'cart.live.spec.ts'],
  outputDir: path.resolve('../../logs/gin-migration/admin-live'),
})
