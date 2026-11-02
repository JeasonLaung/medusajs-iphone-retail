// Execute the original pure helper; no database, provider or credentials.
import {readFile,writeFile} from 'node:fs/promises'
import {createHash} from 'node:crypto'
import {fileURLToPath} from 'node:url'
import {toOptionValues} from '../../medusa-store/apps/backend/src/search/helpers/option-values.ts'

const source=new URL('../../medusa-store/apps/backend/src/search/helpers/option-values.ts',import.meta.url)
const baseline=new URL('../backend/internal/catalog/testdata/search-option-values.json',import.meta.url)
if(process.argv.length!==3)throw new Error('Provide a new evidence output path')
const cases=JSON.parse(await readFile(baseline,'utf8'))
const rows=cases.map(row=>({options:row.options,values:toOptionValues(row.options)}))
const same=JSON.stringify(rows)===JSON.stringify(cases)
await writeFile(process.argv[2],JSON.stringify({at_utc:new Date().toISOString(),source:fileURLToPath(source),source_sha256:createHash('sha256').update(await readFile(source)).digest('hex'),database_accessed:false,cases:rows,matches_checked_in_oracle:same},null,2)+'\n',{flag:'wx'})
if(!same)throw new Error('Original option helper differs from oracle')
console.log(JSON.stringify({cases:rows.length,matches_checked_in_oracle:same,database_accessed:false}))
