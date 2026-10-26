#!/usr/bin/env node
// Read-only, single-snapshot export. The bundle includes private business data:
// write it outside Git, and never print rows or credentials.
const { spawnSync } = require('node:child_process')
const fs = require('node:fs')
const path = require('node:path')
const crypto = require('node:crypto')
const [output, container = 'medusa-trial-postgres-1'] = process.argv.slice(2)
if (!output) throw new Error('Usage: node retail/scripts/export-medusa.cjs <ignored-output.json> [source-container]')
const absolute = path.resolve(output)
const ignored = spawnSync('git', ['check-ignore', '--quiet', absolute])
if (ignored.status !== 0) throw new Error('Output must be Git-ignored because the bundle contains private data')
function sql(query) {
  const result = spawnSync('docker', ['exec', '-i', container, 'psql', '-X', '-q', '-A', '-t', '-v', 'ON_ERROR_STOP=1', '-U', 'medusa', '-d', 'medusa_trial'], {input: query, maxBuffer: 256 * 1024 * 1024})
  if (result.status !== 0) throw new Error('Read-only database export failed: ' + result.stderr.toString())
  return result.stdout.toString().trim()
}
const tables = JSON.parse(sql("SELECT json_agg(table_name ORDER BY table_name) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'"))
const commit = spawnSync('git', ['rev-parse', 'dev'], {encoding:'utf8'}).stdout.trim()
const selects = tables.map(name => {
  const identifier = '"' + name.replaceAll('"', '""') + '"'
  const literal = "'" + name.replaceAll("'", "''") + "'"
  return `SELECT ${literal} AS name, COALESCE((SELECT jsonb_agg(to_jsonb(t)) FROM public.${identifier} t),'[]'::jsonb) AS rows`
}).join(' UNION ALL ')
const query = `BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT jsonb_build_object('format_version',1,'source_commit','${commit}','exported_at',CURRENT_TIMESTAMP,'tables',(SELECT jsonb_object_agg(name,rows) FROM (${selects}) snapshot));
COMMIT;`
const json = sql(query)
const bundle = JSON.parse(json)
fs.mkdirSync(path.dirname(absolute), {recursive:true})
fs.writeFileSync(absolute, json, {mode:0o600, flag:'wx'})
const hash = crypto.createHash('sha256').update(json).digest('hex')
fs.writeFileSync(absolute + '.sha256', hash + '\n', {mode:0o600})
console.log(JSON.stringify({output:absolute,sha256:hash,tables:tables.length,rows:Object.values(bundle.tables).reduce((n,rows)=>n+rows.length,0)}))
