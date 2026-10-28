// Artificial execution of only the installed repository quantity-filter branch.
// No service constructor, application, DB, provider or business data is used.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),vm=require('node:vm'),assert=require('node:assert/strict')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..'),backend=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const resolve=createRequire(backend.resolve('@medusajs/medusa/core-flows'))
const source=path.join(path.dirname(resolve.resolve('@medusajs/pricing')),'repositories/pricing.js')
const content=fs.readFileSync(source,'utf8').replaceAll('\r\n','\n')
const begin=content.indexOf('        if (quantity !== undefined) {'),end=content.indexOf('        if (hasComplexContext)',begin)
assert.ok(begin>0&&end>begin,'Original quantity branch changed')
// This captures the actual query-builder calls, rather than rewriting its
// predicate in JavaScript. The original branch itself performs no I/O.
function capture(quantity){
 const calls=[]
 function builder(){return {
  where(...args){calls.push(['where',...args]);return this},
  orWhere(...args){if(typeof args[0]==='function'){calls.push(['or-group']);args[0].call(builder());calls.push(['end-group'])}else calls.push(['orWhere',...args]);return this},
  andWhere(...args){if(typeof args[0]==='function'){calls.push(['and-group']);args[0].call(builder());calls.push(['end-group'])}else calls.push(['andWhere',...args]);return this},
  whereNull(...args){calls.push(['whereNull',...args]);return this},
  orWhereNull(...args){calls.push(['orWhereNull',...args]);return this},
 }}
 vm.runInNewContext(content.slice(begin,end),{quantity,query:builder()},{timeout:1000})
 return calls
}
const omitted=capture(undefined),one=capture(1),two=capture(2)
assert.deepEqual(omitted,[['and-group'],['where','price.min_quantity','<=',1],['orWhereNull','price.min_quantity'],['end-group']])
assert.ok(one.some(row=>row.includes('price.max_quantity')))
assert.ok(two.some(row=>row.includes(2)))
const text=JSON.stringify({format_version:1,scope:'Actual original SQL-builder quantity branch only; not a database or full shipping acceptance',source_file:path.relative(root,source).replaceAll('\\','/'),source_sha256:crypto.createHash('sha256').update(content).digest('hex'),cases:[{name:'omitted',calls:omitted},{name:'one',calls:one},{name:'two',calls:two}]},null,2)+'\n'
const target=path.join(root,'docs/migration/fixtures/pricing-quantity.json')
assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'),'Unknown option')
if(process.argv[2])assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),text,'Original quantity evidence drift')
else fs.writeFileSync(target,text)
console.log(JSON.stringify({original_quantity_cases:3,application_started:false}))
