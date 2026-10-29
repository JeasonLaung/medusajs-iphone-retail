const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict')
const root=path.resolve(__dirname,'../..'),store=path.join(root,'medusa-store/node_modules/.pnpm')
const file=path.join(store,fs.readdirSync(store).find(p=>p.startsWith('@medusajs+core-flows@')),'node_modules/@medusajs/core-flows/dist/cart/steps/get-promotion-codes-to-apply.js')
const source=fs.readFileSync(file,'utf8'),start=source.indexOf('async (data, { container }) => {'),end=source.lastIndexOf('\n});')
assert.ok(start>0&&end>start)
const framework=require('node:module').createRequire(file)('@medusajs/framework/utils')
const run=new Function('utils_1','workflows_sdk_1','return '+source.slice(start,end+2))(framework,{StepResponse:class{constructor(value){this.value=value}}})
const cases=[]
async function add(name,existing,requested,known,action,fail=false){
 const input={cart:{items:[{adjustments:existing.map(code=>({code}))}]},promo_codes:requested,action}
 let expected=null,error=false
 try{expected=(await run(input,{container:{resolve:()=>({graph:async()=>({data:known.map(code=>({code}))})})}})).value}catch(e){error=true}
 assert.equal(error,fail,name)
 cases.push({name,existing,requested,known,action:action??'',expected,error})
}
async function main(){
 await add('default-add',['OLD','OLD','',null],['NEW'],['NEW'])
 await add('add-stable-dedup',['A','B'],['B','C','C'],['B','C'],'add')
 await add('replace',['OLD'],['B','A','B'],['A','B'],'replace')
 await add('replace-empty',['OLD'],[],[],'replace')
 await add('remove',['A','B','C'],['B'],[],'remove')
 await add('remove-unknown',['A'],['unknown'],[],'remove')
 await add('remove-all',['A','A'],['A'],[],'remove')
 await add('keep-existing-without-resolve',['OLD'],[],[],'add')
 await add('unknown-add',['OLD'],['unknown'],[],'add',true)
 await add('unknown-replace',['OLD'],['unknown'],[],'replace',true)
 await add('case-sensitive',['A'],['save'],['SAVE'],'add',true)
 await add('whitespace-preserved',[],[' SAVE '],[' SAVE '],'add')
 const target=path.join(root,'docs/migration/fixtures/cart-promotion-codes.json'),content=JSON.stringify({format_version:1,scope:'Original selection step with artificial query only; no cart mutation',source_file:path.relative(root,file).replaceAll('\\','/'),source_sha256:crypto.createHash('sha256').update(source.replaceAll('\r\n','\n')).digest('hex'),cases},null,2)+'\n'
 assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
 if(process.argv[2])assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),content)
 else fs.writeFileSync(target,content)
 console.log(JSON.stringify({original_code_selection_cases:cases.length,application_started:false}))
}
main().catch(e=>{console.error(e);process.exitCode=1})
