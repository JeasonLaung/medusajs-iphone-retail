import { defineConfig } from '@playwright/test'
import path from 'node:path'

const output = path.resolve('../../logs/gin-migration/admin-ui')
export default defineConfig({
  testDir: './tests', testMatch: 'auth.spec.ts', fullyParallel: false, workers: 1,
  outputDir: output, reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:19200', headless: true, trace: 'off', screenshot: 'off',
    ...(process.env.ADMIN_TEST_BROWSER ? { launchOptions: { executablePath: process.env.ADMIN_TEST_BROWSER } } : {}),
  },
  webServer: { command: 'npm run dev -- --port 19200', url: 'http://127.0.0.1:19200', reuseExistingServer: false, timeout: 60_000 },
})
