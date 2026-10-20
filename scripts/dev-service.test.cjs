const test = require("node:test")
const assert = require("node:assert/strict")
const fs = require("node:fs")
const os = require("node:os")
const path = require("node:path")
const { spawn, spawnSync } = require("node:child_process")

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
async function until(predicate, description) {
  const deadline = Date.now() + 20000
  while (Date.now() < deadline) { if (predicate()) return; await wait(100) }
  assert.fail(`Timed out: ${description}`)
}

test("backend batches writes, pauses reloads, recovers crashes and stops intentionally", { timeout: 60000 }, async () => {
  const fixture = fs.mkdtempSync(path.join(os.tmpdir(), "medusa-dev-service-"))
  const write = (file, content) => {
    const target = path.join(fixture, file)
    fs.mkdirSync(path.dirname(target), { recursive: true })
    fs.writeFileSync(target, content)
  }
  fs.mkdirSync(path.join(fixture, "scripts"))
  fs.copyFileSync(path.join(__dirname, "dev-service.cjs"), path.join(fixture, "scripts/dev-service.cjs"))
  const backend = "medusa-store/apps/backend"
  write(`${backend}/package.json`, "{}")
  write(`${backend}/src/api/probe/route.ts`, "initial")
  write(`${backend}/node_modules/@medusajs/cli/cli.js`, `
    const fs = require('node:fs')
    const path = require('node:path')
    if (process.argv[2] === 'lint') process.exit(0)
    const event = (value) => fs.appendFileSync(path.join(process.cwd(), 'events'), value + '\\n')
    event('start:' + process.pid)
    console.error('Server is ready')
    process.on('message', (msg) => {
      event('hmr')
      process.send({ type: 'hmr-result', success: !fs.existsSync('fail-hmr') })
    })
    setInterval(() => { if (fs.existsSync('crash')) { fs.unlinkSync('crash'); process.exit(2) } }, 100)
  `)
  // Use the actual installed Medusa watcher, but a fixture CLI instead of a real DB.
  const medusa = path.resolve(__dirname, "../medusa-store/apps/backend/node_modules/@medusajs/medusa")
  fs.symlinkSync(fs.realpathSync(medusa), path.join(fixture, backend, "node_modules/@medusajs/medusa"), process.platform === "win32" ? "junction" : "dir")
  const runner = path.join(fixture, "scripts/dev-service.cjs")
  const child = spawn(process.execPath, [runner, "backend"], { env: { ...process.env, DEV_SERVICE_PORT: "49371" }, stdio: ["ignore", "pipe", "pipe"] })
  let output = ""
  child.stdout.on("data", (data) => { output += data })
  child.stderr.on("data", (data) => { output += data })
  const events = () => {
    try { return fs.readFileSync(path.join(fixture, backend, "events"), "utf8").trim().split("\n") } catch { return [] }
  }
  const starts = () => events().filter((event) => event.startsWith("start:")).length
  const request = (command) => assert.equal(spawnSync(process.execPath, [runner, "backend", command]).status, 0)
  try {
    await until(() => starts() === 1, `initial start ${output}`)
    await wait(1000)
    request("pause")
    write(`${backend}/src/api/probe/route.ts`, "one")
    write(`${backend}/src/api/second/route.ts`, "two")
    await wait(3500)
    assert.equal(events().filter((event) => event === "hmr").length, 0)
    request("resume")
    await until(() => events().filter((event) => event === "hmr").length === 2, "resume HMR")
    assert.equal(starts(), 1, "HMR keeps server process")
    write(`${backend}/src/__tests__/ignored.unit.spec.ts`, "test")
    await wait(3500)
    assert.equal(starts(), 1)
    assert.equal(events().length, 3, "test files do not reload")
    write(`${backend}/src/lib/a.ts`, "a")
    await wait(300)
    write(`${backend}/src/lib/b.ts`, "b")
    await until(() => starts() === 2, "one restart for two library writes")
    await wait(3500)
    assert.equal(starts(), 2)
    write(`${backend}/fail-hmr`, "1")
    write(`${backend}/src/api/probe/route.ts`, "three")
    await until(() => starts() === 3, "failed HMR falls back to restart")
    write(`${backend}/crash`, "1")
    await until(() => starts() === 4, "automatic crash recovery")
    const duplicate = spawnSync(process.execPath, [runner, "backend"], { env: { ...process.env, DEV_SERVICE_PORT: "49371" } })
    assert.notEqual(duplicate.status, 0, "duplicate supervisor is rejected")
    request("stop")
    await until(() => child.exitCode !== null, "intentional stop")
    assert.equal(child.exitCode, 0, output)
    assert.equal(starts(), 4, "intentional stop does not restart")
    assert.equal(fs.existsSync(path.join(fixture, "logs/dev-backend.lock")), false)
  } finally {
    if (child.exitCode === null) {
      request("stop")
      await until(() => child.exitCode !== null, "cleanup")
    }
    // Only remove this verified temporary fixture, never a project checkout.
    assert.ok(fixture.startsWith(path.join(os.tmpdir(), "medusa-dev-service-")))
    fs.rmSync(fixture, { recursive: true, force: true })
  }
})
