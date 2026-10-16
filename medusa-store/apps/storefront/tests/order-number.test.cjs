const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const path = require("node:path")
const ts = require("typescript")

const source = fs.readFileSync(path.join(__dirname, "../src/lib/util/order-number.ts"), "utf8")
const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
const loaded = { exports: {} }
new Function("module", "exports", code)(loaded, loaded.exports)
const { getOrderNumber } = loaded.exports

test("19-digit order numbers retain every digit as a string", () => {
  assert.equal(getOrderNumber({ display_id: 36, custom_display_id: "2026101612345612345" }), "2026101612345612345")
})

test("historical orders keep their original number", () => {
  for (const custom_display_id of [undefined, null, ""]) {
    assert.equal(getOrderNumber({ display_id: 36, custom_display_id }), "36")
  }
})
