const { test } = require("node:test")
const assert = require("node:assert/strict")
const { affectedServices } = require("./deploy-changes.cjs")

test("deploy only the changed application, including Admin and static assets", () => {
  assert.deepEqual(affectedServices(["medusa-store/apps/backend/src/admin/page.tsx"]), ["backend"])
  assert.deepEqual(affectedServices(["medusa-store/apps/backend/static/mobile-intake/app.js"]), ["backend"])
  assert.deepEqual(affectedServices(["medusa-store/apps/storefront/src/page.tsx"]), ["storefront"])
})
test("shared dependencies and deployment configuration deploy both", () => {
  for (const path of ["medusa-store/pnpm-lock.yaml", "medusa-store/shared/copy.json", "deploy/backend.Dockerfile", ".dockerignore", ".github/workflows/deploy.yaml"]) {
    assert.deepEqual(affectedServices([path]), ["backend", "storefront"])
  }
})
test("documentation and local development infrastructure do not deploy", () => {
  assert.deepEqual(affectedServices(["README.md", "docs/guide.md", "compose.yaml", "scripts/dev-service.cjs"]), [])
})
test("combined changes produce one entry per service", () => {
  assert.deepEqual(affectedServices(["medusa-store/apps/storefront/a", "medusa-store/apps/backend/b", "medusa-store/pnpm-lock.yaml"]), ["backend", "storefront"])
})
