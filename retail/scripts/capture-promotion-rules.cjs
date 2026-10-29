// Actual installed pure promotion predicate, artificial context only.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..'),backend=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const base=fs.readdirSync(path.join(root,'medusa-store/node_modules/.pnpm')).find(p=>p.startsWith('@medusajs+promotion@'))
assert.ok(base)
const source=path.join(root,'medusa-store/node_modules/.pnpm',base,'node_modules/@medusajs/promotion/dist/utils/validations/promotion-rule.js')
const {areRulesValidForContext}=require(source)
const cases=[]
function add(name,context,attribute,operator,values,scope,expected) {
 const rules=attribute===null?[]:[{attribute,operator,values:values.map(value=>({value}))}]
 const before=JSON.stringify({context,rules}),got=areRulesValidForContext(rules,context,scope||undefined)
 assert.equal(got,expected,name);assert.equal(JSON.stringify({context,rules}),before)
 cases.push({name,context,rules,scope,expected:got})
}
add('no-rules',{},null,'',[],'',true)
for(const [name,value,op,expected] of [
 ['eq-scalar','a','eq',true],['eq-all',['a','b'],'eq',true],['eq-partial',['a','c'],'eq',false],
 ['in-any',['a','c'],'in',true],['in-none',['c'],'in',false],['ne-all',['c','d'],'ne',true],['ne-overlap',['c','a'],'ne',false],
 ['empty-eq',[],'eq',false],['empty-in',[],'in',false],['empty-ne',[],'ne',true],
 ['number-string',1,'eq',true],['bool-string',false,'eq',true],['null-string',null,'eq',true],
 ['nested-array', [['a','b']],'eq',true],['object-string',{},'eq',true]
]) add(name,{x:value},'x',op,['a','b','1','false','null','a,b','[object Object]'],'',expected)
add('missing-eq',{},'x','eq',['a'],'',false)
add('missing-ne',{},'x','ne',['a'],'',true)
add('missing-literal',{},'x','eq',['undefined'],'',true)
add('item-prefix',{product:{id:'p'}},'items.product.id','eq',['p'],'items',true)
add('shipping-prefix',{type:'express'},'shipping_methods.type','eq',['express'],'shipping_methods',true)
add('array-path',{items:[{product:{id:'p'}},{product:{id:'q'}}]},'items.product.id','eq',['p','q'],'',true)
add('scalar-path',{x:'a'},'x.remaining.path','eq',['a'],'',true)
add('mixed-values',{x:'a'},'x','eq',[1,null,false,'a'],'',true)
add('empty-string',{x:''},'x','eq',[''],'',true)
for(const [op,value,expected] of [['gt','10.000000000000000000001',true],['gte','10',true],['lt','9.9',true],['lte','10',true],['gt','10',false],['gt',['11','9'],false],['gt',['11','12'],true]]) add('range-'+op+'-'+cases.length,{x:value},'x',op,['10'],'',expected)
add('range-any-threshold',{x:'15'},'x','gt',['10','20'],'',true)
add('large-exact',{x:'9007199254740993'},'x','gt',['9007199254740992'],'',true)
const target=path.join(root,'docs/migration/fixtures/promotion-rules.json')
const content=JSON.stringify({format_version:1,scope:'Original promotion predicate only, not allocation/budget/SQL or checkout',source_file:path.relative(root,source).replaceAll('\\','/'),source_sha256:crypto.createHash('sha256').update(fs.readFileSync(source,'utf8').replaceAll('\r\n','\n')).digest('hex'),cases},null,2)+'\n'
assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
if(process.argv[2])assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),content)
else fs.writeFileSync(target,content)
console.log(JSON.stringify({original_promotion_rule_cases:cases.length,application_started:false}))
