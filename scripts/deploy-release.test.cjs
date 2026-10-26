// Run on Linux: node --test scripts/deploy-release.test.cjs
const { test } = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const os = require("node:os")
const path = require("node:path")
const { spawnSync } = require("node:child_process")

const sha = "a".repeat(40)
const prefix = "ghcr.io/acme/retail"
const oldBackend = `${prefix}/backend:old`
const oldStorefront = `${prefix}/storefront:old`

function release({ backend = true, storefront = false, fail = "" } = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "deploy-test-"))
  fs.mkdirSync(path.join(root, "bin"))
  for (const file of [".env", "backend.env", "storefront.env"]) fs.writeFileSync(path.join(root, file), "")
  fs.writeFileSync(path.join(root, ".images.env"), `BACKEND_IMAGE=${oldBackend}\nSTOREFRONT_IMAGE=${oldStorefront}\n`)
  fs.writeFileSync(path.join(root, "bin/docker"), `#!/usr/bin/env bash
set -eu
printf '%s|%s|%s\\n' "$*" "$BACKEND_IMAGE" "$STOREFRONT_IMAGE" >> "$DEPLOY_ROOT/calls"
if [[ "$*" == *' ps -a -q '* ]]; then printf 'existing-container\\n'; fi
if [[ "$MOCK_FAIL" == migration && "$*" == *' db:migrate' ]]; then exit 1; fi
if [[ "$MOCK_FAIL" == backend && "$*" == *' up -d --no-deps '* && "$*" == *' backend' && "$BACKEND_IMAGE" == *':${sha}' ]]; then exit 1; fi
if [[ "$MOCK_FAIL" == storefront && "$*" == *' up -d --no-deps '* && "$*" == *' storefront' && "$STOREFRONT_IMAGE" == *':${sha}' ]]; then exit 1; fi
`, { mode: 0o755 })
  const result = spawnSync("bash", [path.resolve(__dirname, "../deploy/deploy.sh"), root, prefix, sha, String(backend), String(storefront)], {
    encoding: "utf8",
    env: { ...process.env, PATH: `${root}/bin:${process.env.PATH}`, MOCK_FAIL: fail },
  })
  const state = fs.readFileSync(path.join(root, ".images.env"), "utf8")
  const calls = fs.readFileSync(path.join(root, "calls"), "utf8").trim().split("\n")
  fs.rmSync(root, { recursive: true, force: true })
  return { result, state, calls }
}

test("backend release leaves storefront untouched and migrates exactly once", () => {
  const { result, state, calls } = release()
  assert.equal(result.status, 0, result.stderr)
  assert.match(state, new RegExp(`BACKEND_IMAGE=${prefix}/backend:${sha}`))
  assert.ok(state.includes(`STOREFRONT_IMAGE=${oldStorefront}`))
  assert.equal(calls.filter(call => call.includes(" db:migrate")).length, 1)
  assert.ok(calls.find(call => call.includes(" db:migrate")).includes("--user 0:0 backend"))
  assert.ok(!calls.some(call => / (pull|up) .* storefront\|/.test(call)))
})
test("frontend release does not run backend migrations", () => {
  const { result, state, calls } = release({ backend: false, storefront: true })
  assert.equal(result.status, 0, result.stderr)
  assert.ok(state.includes(`BACKEND_IMAGE=${oldBackend}`))
  assert.ok(state.includes(`STOREFRONT_IMAGE=${prefix}/storefront:${sha}`))
  assert.ok(!calls.some(call => call.includes("db:migrate")))
})
test("migration failure preserves running backend and saved image", () => {
  const { result, state, calls } = release({ fail: "migration" })
  assert.notEqual(result.status, 0)
  assert.ok(state.includes(`BACKEND_IMAGE=${oldBackend}`))
  assert.ok(!calls.some(call => call.includes(" up -d --no-deps ")))
})
test("backend health failure restores the previous backend image", () => {
  const { result, state, calls } = release({ fail: "backend" })
  assert.notEqual(result.status, 0)
  assert.ok(state.includes(`BACKEND_IMAGE=${oldBackend}`))
  const updates = calls.filter(call => call.includes(" up -d --no-deps "))
  assert.equal(updates.length, 2)
  assert.ok(updates[1].includes(`|${oldBackend}|`))
})
test("frontend failure preserves the independently completed backend release", () => {
  const { result, state, calls } = release({ storefront: true, fail: "storefront" })
  assert.notEqual(result.status, 0)
  assert.ok(state.includes(`BACKEND_IMAGE=${prefix}/backend:${sha}`))
  assert.ok(state.includes(`STOREFRONT_IMAGE=${oldStorefront}`))
  assert.ok(calls.at(-1).endsWith(`|${oldStorefront}`))
})
