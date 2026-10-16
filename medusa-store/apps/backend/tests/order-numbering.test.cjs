const { test } = require("node:test")
const assert = require("node:assert/strict")
const { execFile, execFileSync } = require("node:child_process")
const path = require("node:path")
const { promisify } = require("node:util")

const cwd = path.resolve(__dirname, "../../../..")
const args = ["compose", "exec", "-T", "postgres", "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "medusa", "-d", "medusa_trial"]
const sql = (query) => execFileSync("docker", [...args, "-c", query], { cwd, encoding: "utf8" }).trim()

test("five-digit order numbers are unique across concurrent database connections", async () => {
  const table = `order_number_test_${process.pid}`
  sql(`CREATE TABLE ${table} (id text PRIMARY KEY, custom_display_id text UNIQUE);
    CREATE TRIGGER number_test BEFORE INSERT ON ${table}
    FOR EACH ROW EXECUTE FUNCTION assign_timestamp_order_number();`)
  try {
    await Promise.all(Array.from({ length: 8 }, (_, worker) =>
      promisify(execFile)("docker", [...args, "-c",
        `INSERT INTO ${table} (id) SELECT '${worker}_' || n FROM generate_series(1, 125) n;`
      ], { cwd })
    ))
    assert.equal(sql(`SELECT count(*), count(DISTINCT custom_display_id),
      bool_and(custom_display_id ~ '^[0-9]{19}$') FROM ${table};`), "1000|1000|t")
  } finally {
    sql(`DROP TABLE ${table};`)
  }
})

test("suffix overflow and a clock behind the allocator advance without duplicates", () => {
  const output = sql(`BEGIN;
    CREATE TEMP TABLE number_boundary (id integer, custom_display_id text);
    CREATE TRIGGER number_test BEFORE INSERT ON number_boundary
      FOR EACH ROW EXECUTE FUNCTION assign_timestamp_order_number();
    UPDATE order_number_clock SET last_tick =
      (floor(extract(epoch FROM clock_timestamp()))::bigint + 3600) * 100000 + 99998;
    INSERT INTO number_boundary (id) VALUES (1), (2), (3);
    SELECT right(custom_display_id, 5) FROM number_boundary ORDER BY id;
    SELECT count(DISTINCT custom_display_id) FROM number_boundary;
    ROLLBACK;`)
  assert.equal(output, "99999\n00000\n00001\n3")
})

test("real order inserts persist the number and failed transactions restore the clock", () => {
  const output = sql(`BEGIN;
    CREATE TEMP TABLE previous_clock AS SELECT last_tick FROM order_number_clock;
    SAVEPOINT before_order;
    INSERT INTO "order" (id, currency_code)
      VALUES ('order_number_rollback_test_${process.pid}', 'eur');
    SELECT custom_display_id ~ '^[0-9]{19}$' FROM "order"
      WHERE id = 'order_number_rollback_test_${process.pid}';
    ROLLBACK TO SAVEPOINT before_order;
    SELECT c.last_tick = p.last_tick FROM order_number_clock c, previous_clock p;
    ROLLBACK;`)
  assert.equal(output, "t\nt")
})
