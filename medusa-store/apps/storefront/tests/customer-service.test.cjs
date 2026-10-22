const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")
function load(name, imports = {}) {
  const source = fs.readFileSync(path.join(__dirname, `../src/modules/customer-service/${name}`), "utf8")
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX } }).outputText
  const loaded = { exports: {} }
  new Function("module", "exports", "require", code)(loaded, loaded.exports, id => imports[id] || require(id))
  return loaded.exports
}
const helpers = load("contact.ts")
const i18n = load("i18n.ts")
const contact = { id: "one", name: "Sales", department: "pre_sales", whatsapp: "+8613800000000", email: "sales@example.test", timezone: "Asia/Shanghai", days: [1, 2, 3, 4, 5], start: "09:00", end: "18:00", availability: "schedule" }
test("service hours use the configured timezone with inclusive start and exclusive end", () => {
  assert.equal(helpers.isWithinServiceHours(contact, new Date("2026-10-23T00:59:00Z")), false)
  assert.equal(helpers.isWithinServiceHours(contact, new Date("2026-10-23T01:00:00Z")), true)
  assert.equal(helpers.isWithinServiceHours(contact, new Date("2026-10-23T10:00:00Z")), false)
  assert.equal(helpers.isWithinServiceHours(contact, new Date("2026-10-24T02:00:00Z")), false)
})
test("DST and midnight are interpreted in the contact's local timezone", () => {
  const european = { ...contact, timezone: "Europe/Copenhagen", days: [0] }
  assert.equal(helpers.isWithinServiceHours(european, new Date("2026-10-25T08:00:00Z")), true)
  assert.equal(helpers.isWithinServiceHours({ ...contact, days: [6], start: "00:00", end: "01:00" }, new Date("2026-10-23T16:00:00Z")), true)
  assert.equal(helpers.isWithinServiceHours({ ...contact, timezone: "invalid" }, new Date()), false)
})
test("WhatsApp links strip the plus sign and reject URLs or invalid numbers", () => {
  assert.equal(helpers.whatsappLink(contact.whatsapp), "https://wa.me/8613800000000")
  for (const value of ["javascript:alert(1)", "https://example.test", "123", ""]) assert.equal(helpers.whatsappLink(value), null)
})
test("both languages render multiple contacts, both teams and contact links", () => {
  const React = require("react")
  const { renderToStaticMarkup } = require("react-dom/server")
  const Component = load("index.tsx", { "./contact": helpers, "./i18n": i18n, "./styles.module.css": new Proxy({}, { get: (_, key) => key === "__esModule" ? false : key }) }).default
  for (const locale of ["en", "zh"]) {
    const copy = i18n.getServiceCopy(locale)
    const html = renderToStaticMarkup(React.createElement(Component, { locale, failed: false, contacts: [contact, { ...contact, id: "two", name: "Second", availability: "online" }, { ...contact, id: "after", name: "Support", department: "after_sales", availability: "offline" }] }))
    for (const text of [copy.pre_sales, copy.after_sales, copy.online, copy.offline, "Second", "Support", "Asia/Shanghai", "https://wa.me/8613800000000", "mailto:sales%40example.test"]) assert.ok(html.includes(text), text)
    assert.equal((html.match(/<li\b/g) || []).length, 3)
  }
})
test("Chinese and English resources contain the same keys", () => {
  assert.deepEqual(Object.keys(i18n.getServiceCopy("zh")), Object.keys(i18n.getServiceCopy("en")))
})
