const fs = require("node:fs")
const path = require("node:path")
const { fork, spawn } = require("node:child_process")
const { createRequire } = require("node:module")
const net = require("node:net")

const root = path.resolve(__dirname, "..")
const logs = path.join(root, "logs")
const app = process.argv[2]
const command = process.argv[3]
if (!["backend", "storefront"].includes(app)) throw new Error("Use backend or storefront")
fs.mkdirSync(logs, { recursive: true })
const control = (suffix) => path.join(logs, `dev-${app}.${suffix}`)
const log = (message) => console.log(`[dev:${app}] ${new Date().toISOString()} ${message}`)

if (command) {
  if (command === "pause") fs.writeFileSync(control("pause"), String(Date.now()))
  else if (command === "resume") fs.rmSync(control("pause"), { force: true })
  else if (["restart", "stop"].includes(command)) fs.writeFileSync(control(command), String(Date.now()))
  else throw new Error("Use pause, resume, restart or stop")
  log(`${command} requested`)
} else {
  main().catch((error) => { console.error(error); process.exitCode = 1 })
}

async function main() {
  const directory = path.join(root, "medusa-store", "apps", app)
  const requireApp = createRequire(path.join(directory, "package.json"))
  const port = Number(process.env.DEV_SERVICE_PORT || (app === "backend" ? 9001 : 8000))
  if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error("Invalid DEV_SERVICE_PORT")
  // Never launch a competing instance, including one in its startup phase.
  let lock
  try { lock = fs.openSync(control("lock"), "wx") } catch (error) {
    if (error.code !== "EEXIST") throw error
    const owner = Number(fs.readFileSync(control("lock"), "utf8"))
    try { process.kill(owner, 0) } catch (error) {
      if (error.code !== "ESRCH") throw error
      fs.unlinkSync(control("lock"))
      return main()
    }
    throw new Error(`${app} supervisor already running (PID ${owner})`)
  }
  fs.writeFileSync(lock, String(process.pid))
  const release = () => { fs.closeSync(lock); fs.rmSync(control("lock"), { force: true }) }
  process.once("exit", release)
  await new Promise((resolve, reject) => {
    const probe = net.createServer()
    probe.once("error", reject)
    probe.listen(port, "0.0.0.0", () => probe.close(resolve))
  })
  for (const suffix of ["stop", "restart"]) fs.rmSync(control(suffix), { force: true })
  const cli = app === "backend" ? requireApp.resolve("@medusajs/cli/cli.js") : requireApp.resolve("next/dist/bin/next")
  if (app === "backend") {
    const result = await new Promise((resolve, reject) => {
      const lint = spawn(process.execPath, [cli, "lint", "--quiet"], { cwd: directory, stdio: "inherit", windowsHide: true })
      lint.once("error", reject)
      lint.once("exit", resolve)
    })
    if (result !== 0) throw new Error("Backend lint failed; fix errors before starting development")
  }
  const args = app === "backend" ? ["start", "--host", "0.0.0.0", "--port", String(port)] : ["dev", "--turbopack", "-H", "127.0.0.1", "-p", String(port)]
  let child, stopping = false, busy = false, ready = false, failures = 0, nextStart = 0
  let lastChange = 0, startedAt = 0
  const pending = new Map()
  const paused = () => {
    try { return Date.now() - Number(fs.readFileSync(control("pause"), "utf8")) < 30 * 60 * 1000 } catch { return false }
  }
  const start = () => {
    ready = false
    const started = Date.now()
    startedAt = started
    const instance = fork(cli, args, {
      cwd: directory,
      env: { ...process.env, NODE_ENV: "development", MEDUSA_HMR_ENABLED: "true", MEDUSA_FF_BACKEND_HMR: "true" },
      stdio: ["ignore", "pipe", "pipe", "ipc"],
    })
    child = instance
    log(`started PID ${instance.pid}`)
    let startupOutput = ""
    const forward = (stream) => (data) => {
      stream.write(data)
      startupOutput = (startupOutput + data.toString()).slice(-4096)
      if (/Server is ready|Ready in/.test(startupOutput)) { ready = true; failures = 0 }
    }
    instance.stdout.on("data", forward(process.stdout))
    instance.stderr.on("data", forward(process.stderr))
    instance.on("error", (error) => log(`child error: ${error.message}`))
    instance.once("exit", (code, signal) => {
      if (child !== instance) return
      child = undefined
      ready = false
      failures = Date.now() - started > 60000 ? 1 : failures + 1
      const delay = Math.min(30000, 1000 * 2 ** Math.min(failures, 5))
      nextStart = Date.now() + delay
      log(`exited (${code ?? signal}); retry in ${delay / 1000}s`)
    })
  }
  const terminate = async () => {
    const instance = child
    if (!instance) return
    child = undefined
    ready = false
    if (instance.exitCode !== null || instance.signalCode !== null) return
    const exited = new Promise((resolve) => instance.once("exit", resolve))
    if (process.platform === "win32") {
      await new Promise((resolve) => {
        const killer = spawn("taskkill", ["/PID", String(instance.pid), "/F", "/T"], { windowsHide: true, stdio: "ignore" })
        killer.once("error", resolve)
        killer.once("exit", resolve)
      })
    } else instance.kill("SIGTERM")
    await Promise.race([exited, new Promise((resolve) => setTimeout(resolve, 5000).unref())])
    if (instance.exitCode === null && instance.signalCode === null) {
      instance.kill("SIGKILL")
      await exited
    }
  }
  const reload = (file, action) => new Promise((resolve) => {
    const instance = child
    if (!instance?.connected) return resolve(false)
    const finish = (success) => {
      clearTimeout(timeout)
      instance.off("message", onMessage)
      instance.off("exit", onExit)
      resolve(success)
    }
    const onMessage = (message) => { if (message?.type === "hmr-result") finish(message.success === true) }
    const onExit = () => finish(false)
    const timeout = setTimeout(() => finish(false), 30000)
    instance.on("message", onMessage)
    instance.once("exit", onExit)
    instance.send({ type: "hmr-reload", action, file: path.join(directory, file), rootDirectory: directory }, (error) => { if (error) finish(false) })
  })
  let watcher
  if (app === "backend") {
    // Reuse Medusa's installed watcher; no additional dependency or node_modules edits.
    const requireMedusa = createRequire(requireApp.resolve("@medusajs/medusa"))
    const chokidar = requireMedusa("chokidar")
    watcher = chokidar.watch(["src", "medusa-config.ts", ".env", ".env.local", "package.json"], {
      cwd: directory, ignoreInitial: true,
      ignored: /(^|[/\\])(admin|__tests__|scripts|migration-scripts|migrations)([/\\]|$)|\.(test|spec)\.[cm]?[jt]sx?$|[/\\]\.snapshot/,
      awaitWriteFinish: { stabilityThreshold: 750, pollInterval: 100 },
    })
    watcher.on("all", (action, file) => {
      if (!["add", "change", "unlink"].includes(action)) return
      pending.set(file.replaceAll("\\", "/"), action)
      lastChange = Date.now()
    })
    watcher.on("error", (error) => log(`watcher error: ${error.message}`))
  }
  const shutdown = async () => {
    if (stopping) return
    stopping = true
    clearInterval(timer)
    await watcher?.close()
    await terminate()
    log("stopped")
  }
  process.once("SIGINT", shutdown)
  process.once("SIGTERM", shutdown)
  const timer = setInterval(async () => {
    if (busy || stopping) return
    busy = true
    try {
      if (fs.existsSync(control("stop"))) { fs.rmSync(control("stop"), { force: true }); await shutdown(); return }
      if (fs.existsSync(control("restart"))) {
        fs.rmSync(control("restart"), { force: true })
        pending.set("medusa-config.ts", "change")
        lastChange = 0
      }
      if (paused()) return
      if (!child) {
        if (Date.now() >= nextStart) { pending.clear(); start() }
        return
      }
      if (!ready && Date.now() - startedAt > 120000) {
        log("startup did not finish within 120s; retrying in 30s")
        await terminate()
        nextStart = Date.now() + 30000
        return
      }
      if (!ready || !pending.size || Date.now() - lastChange < 2000) return
      const changes = [...pending]
      pending.clear()
      // Modules/config/shared libraries need a full restart to rebuild their state.
      let hot = app === "backend" && changes.every(([file]) => /^src\/(api|workflows)\//.test(file))
      if (hot) for (const [file, action] of changes) {
        if (!(await reload(file, action))) { hot = false; break }
      }
      if (stopping) return
      if (hot) log(`hot reloaded ${changes.length} file(s)`)
      else { log(`one restart for ${changes.length} file(s)`); await terminate(); if (!stopping) start() }
    } catch (error) { log(`update failed: ${error.message}`) }
    finally { busy = false }
  }, 500)
  start()
}
