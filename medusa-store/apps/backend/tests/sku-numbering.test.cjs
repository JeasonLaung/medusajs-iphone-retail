const { test } = require("node:test")
const assert = require("node:assert/strict")
const { execFile, execFileSync } = require("node:child_process")
const path = require("node:path")
const { promisify } = require("node:util")

const cwd = path.resolve(__dirname, "../../../..")
const args = ["compose", "exec", "-T", "postgres", "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "medusa", "-d", "medusa_trial"]
const sql = (query) => execFileSync("docker", [...args, "-c", query], { cwd, encoding: "utf8" }).trim()

test("ERP numbers use ID + 10000, survive concurrency and preserve custom codes", async () => {
  const schema = `sku_test_${process.pid}`
  const definition = sql("SELECT pg_get_functiondef('assign_erp_variant_sku()'::regprocedure)")
    .replace("public.assign_erp_variant_sku()", `${schema}.assign_erp_variant_sku()`)
    .replaceAll('"sku_number_counter"', `${schema}.sku_number_counter`)
  sql(`CREATE SCHEMA ${schema};
    CREATE TABLE ${schema}.sku_number_counter (id boolean PRIMARY KEY, last_id bigint NOT NULL);
    INSERT INTO ${schema}.sku_number_counter VALUES (true, 0);
    CREATE TABLE ${schema}.variants (id text PRIMARY KEY, sku text UNIQUE);
    ${definition};
    CREATE TRIGGER sku_test BEFORE INSERT OR UPDATE OF sku ON ${schema}.variants
      FOR EACH ROW EXECUTE FUNCTION ${schema}.assign_erp_variant_sku();`)
  try {
    assert.equal(sql(`INSERT INTO ${schema}.variants (id, sku) VALUES
      ('a', NULL), ('b', ''), ('c', E' \t\n'), ('d', 'CUSTOM-PHONE') RETURNING sku;`),
    "ERP10001\nERP10002\nERP10003\nCUSTOM-PHONE")
    assert.equal(sql(`UPDATE ${schema}.variants SET sku = '' WHERE id = 'a' RETURNING sku;`), "ERP10001")
    assert.equal(sql(`INSERT INTO ${schema}.variants VALUES ('explicit', 'ERP11000');
      INSERT INTO ${schema}.variants (id) VALUES ('after-explicit') RETURNING sku;`), "ERP11001")
    // Committed deletion must not recycle an assigned number.
    assert.equal(sql(`DELETE FROM ${schema}.variants WHERE id = 'after-explicit';
      INSERT INTO ${schema}.variants (id) VALUES ('after-delete') RETURNING sku;`), "ERP11002")
    // Allocation has no five-digit truncation at the boundary.
    assert.equal(sql(`BEGIN;
      UPDATE ${schema}.sku_number_counter SET last_id = 89999;
      INSERT INTO ${schema}.variants (id) VALUES ('boundary') RETURNING sku;
      ROLLBACK;`), "ERP100000")
    const before = sql(`SELECT last_id FROM ${schema}.sku_number_counter`)
    sql(`BEGIN; INSERT INTO ${schema}.variants (id) VALUES ('rollback'); ROLLBACK;`)
    assert.equal(sql(`SELECT last_id FROM ${schema}.sku_number_counter`), before)
    await Promise.all(Array.from({ length: 6 }, (_, worker) =>
      promisify(execFile)("docker", [...args, "-c",
        `INSERT INTO ${schema}.variants (id) SELECT '${worker}_' || n FROM generate_series(1, 30) n;`
      ], { cwd })
    ))
    assert.equal(sql(`SELECT count(*), count(DISTINCT sku), bool_and(sku ~ '^ERP[0-9]+$')
      FROM ${schema}.variants WHERE id ~ '^[0-9]+_';`), "180|180|t")
  } finally {
    sql(`DROP SCHEMA ${schema} CASCADE;`)
  }
})

test("real product variants get defaults and roll back without consuming IDs", () => {
  const result = sql(`BEGIN;
    CREATE TEMP TABLE previous_sku_counter AS SELECT last_id FROM sku_number_counter;
    SAVEPOINT before_variant;
    INSERT INTO product_variant (id, title)
      VALUES ('variant_sku_rollback_test_${process.pid}', 'SKU rollback test');
    SELECT sku = 'ERP' || (previous.last_id + 10001)::text
      FROM product_variant, previous_sku_counter previous
      WHERE id = 'variant_sku_rollback_test_${process.pid}';
    ROLLBACK TO SAVEPOINT before_variant;
    SELECT current.last_id = previous.last_id FROM sku_number_counter current, previous_sku_counter previous;
    ROLLBACK;`)
  assert.equal(result, "t\nt")
})
