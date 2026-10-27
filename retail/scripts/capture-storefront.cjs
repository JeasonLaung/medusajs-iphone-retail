// Read-only browser baseline. Screenshots and runtime details must stay Git-ignored.
const fs = require('node:fs')
const path = require('node:path')
const { createRequire } = require('node:module')
const { spawnSync } = require('node:child_process')
const crypto = require('node:crypto')

const root = path.resolve(__dirname, '../..')
const options = Object.fromEntries(process.argv.slice(2).reduce((pairs, item, index, all) => {
  if (item.startsWith('--')) pairs.push([item.slice(2), all[index + 1]])
  return pairs
}, []))
const out = path.resolve(root, options.output || 'logs/gin-migration/storefront-baseline')
if (spawnSync('git', ['check-ignore', '--quiet', path.join(out, 'capture.json')], { cwd: root }).status !== 0) {
  throw new Error('Capture output must be Git-ignored')
}
const base = new URL(options.base || 'http://127.0.0.1:8000')
if (!['127.0.0.1', 'localhost'].includes(base.hostname)) throw new Error('Only local baseline capture is supported')
const localRequire = options['playwright-root']
  ? createRequire(path.join(path.resolve(options['playwright-root']), 'package.json'))
  : require
const { chromium } = localRequire('playwright')

const cases = [
  ['home', ''], ['store', '/store'], ['about', '/about'], ['help', '/help'],
  ['blog', '/blog'], ['article', '/blog/choose-your-storage'], ['reviews', '/reviews'],
  ['sell', '/sell'], ['ui', '/ui'], ['cart-empty', '/cart'], ['account-logged-out', '/account'],
  ['verify-missing-token', '/verify-account'], ['payment-result-missing-reference', '/payment-result'],
]
if (options['product-handle']) {
  const handle = options['product-handle']
  if (!/^[a-z0-9][a-z0-9_-]{0,159}$/.test(handle)) throw new Error('Invalid public product handle')
  cases.push(['product', `/products/${handle}`])
}
const selected = options.cases ? new Set(options.cases.split(',')) : null
const widths = (options.widths || '390,768,1440').split(',').map(Number)
if (widths.some(w => !Number.isInteger(w) || w < 320 || w > 2000)) throw new Error('Invalid viewport')
fs.mkdirSync(out, { recursive: true })

async function main() {
  const browser = await chromium.launch({ headless: true })
  const report = { started_at: new Date().toISOString(), scope: 'public and logged-out states only; not full T003 acceptance', records: [] }
  try {
    for (const language of ['en', 'zh']) {
      for (const width of widths) {
        const context = await browser.newContext({ viewport: { width, height: 900 }, locale: language === 'zh' ? 'zh-CN' : 'en-GB', timezoneId: 'Europe/Copenhagen', reducedMotion: 'reduce' })
        // Do not submit any forms, login, add to cart or invoke write endpoints.
        await context.route('**/*', route => {
          const request = route.request()
          const url = new URL(request.url())
          const readOnlySearch = request.method() === 'POST' && ['127.0.0.1', 'localhost'].includes(url.hostname) && url.pathname === '/store/search'
          return ['GET', 'HEAD', 'OPTIONS'].includes(request.method()) || readOnlySearch ? route.continue() : route.abort('blockedbyclient')
        })
        for (const [name, suffix] of cases) {
          if (selected && !selected.has(name)) continue
          const page = await context.newPage()
          page.setDefaultTimeout(15000)
          const filename = `${name}-${language}-${width}.png`
          const record = { case: name, language, width, path: `/dk-${language}${suffix}`, file: filename, errors: [] }
          const onError = error => { if (record.errors.length < 20) record.errors.push(String(error.message).slice(0, 1500)) }
          page.on('pageerror', onError)
          try {
            const response = await page.goto(new URL(record.path, base).href, { waitUntil: 'domcontentloaded', timeout: 45000 })
            record.status = response?.status()
            record.final_path = new URL(page.url()).pathname
            try { await page.waitForLoadState('networkidle', { timeout: 10000 }) } catch { record.network_idle_timeout = true }
            await page.evaluate(() => Promise.race([document.fonts.ready, new Promise(resolve => setTimeout(resolve, 4000))]))
            await page.evaluate(async () => {
              const height = Math.min(document.documentElement.scrollHeight, 20000)
              for (let y = 0; y < height; y += 700) { scrollTo(0, y); await new Promise(resolve => setTimeout(resolve, 70)) }
              scrollTo(0, 0)
              await Promise.race([Promise.allSettled([...document.images].map(img => img.decode())), new Promise(resolve => setTimeout(resolve, 5000))])
            })
            record.title = await page.title()
            record.document = await page.evaluate(() => ({ lang: document.documentElement.lang, width: document.documentElement.clientWidth, scroll_width: document.documentElement.scrollWidth, height: document.documentElement.scrollHeight, h1: [...document.querySelectorAll('h1')].map(n => n.textContent), canonical: document.querySelector('link[rel=canonical]')?.getAttribute('href'), robots: document.querySelector('meta[name=robots]')?.getAttribute('content'), broken_images: [...document.images].filter(i => i.complete && !i.naturalWidth).length }))
            await page.screenshot({ path: path.join(out, filename), fullPage: true, animations: 'disabled' })
            record.sha256 = crypto.createHash('sha256').update(fs.readFileSync(path.join(out, filename))).digest('hex')
          } catch (error) {
            record.failure = String(error.message).slice(0, 500)
          } finally {
            page.off('pageerror', onError)
            await page.close()
          }
          report.records.push(record)
          fs.writeFileSync(path.join(out, 'capture.json'), JSON.stringify(report, null, 2))
          console.log(`${name} ${language} ${width}: ${record.failure ? 'FAILED' : record.status} errors=${record.errors.length}`)
        }
        await context.close()
      }
    }
  } finally {
    await browser.close()
    report.finished_at = new Date().toISOString()
    fs.writeFileSync(path.join(out, 'capture.json'), JSON.stringify(report, null, 2))
  }
  const failures = report.records.filter(r => r.failure || r.status >= 500)
  console.log(`Captured ${report.records.length - failures.length}/${report.records.length}; report is in the ignored output directory`)
  if (failures.length) process.exitCode = 1
}
main().catch(() => { console.error('Baseline capture failed; inspect local browser/runtime availability'); process.exitCode = 1 })
