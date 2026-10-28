import {defineConfig} from '@playwright/test'
import base from './playwright.live.config'

export default defineConfig({...base,testMatch:'cart-items.live.spec.ts'})
