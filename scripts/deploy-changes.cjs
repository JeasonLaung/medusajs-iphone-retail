const { execFileSync } = require("node:child_process")
const { appendFileSync } = require("node:fs")

function affectedServices(paths) {
  const services = new Set()
  for (const path of paths) {
    if (/^medusa-store\/apps\/backend\//.test(path)) services.add("backend")
    else if (/^medusa-store\/apps\/storefront\//.test(path)) services.add("storefront")
    else if (
      /^(deploy\/|\.dockerignore$|\.github\/workflows\/deploy\.yaml$|scripts\/deploy-changes\.cjs$)/.test(path) ||
      /^medusa-store\/(shared\/|patches\/|package\.json$|pnpm-lock\.yaml$|pnpm-workspace\.yaml$|\.npmrc$|eslint\.config\.ts$|turbo\.json$)/.test(path)
    ) {
      services.add("backend")
      services.add("storefront")
    }
  }
  return ["backend", "storefront"].filter(service => services.has(service))
}

if (require.main === module) {
  const [before, after, selection] = process.argv.slice(2)
  const services = selection
    ? selection === "all" ? ["backend", "storefront"] : [selection]
    : !before || /^0+$/.test(before)
      ? ["backend", "storefront"]
      : affectedServices(execFileSync("git", ["diff", "--name-only", "-z", before, after], { encoding: "utf8" }).split("\0"))
  if (services.some(service => !["backend", "storefront"].includes(service))) throw new Error("Invalid service")
  const output = `services=${JSON.stringify(services)}\nchanged=${services.length > 0}\n`
  if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT, output)
  process.stdout.write(output)
}
module.exports = { affectedServices }
