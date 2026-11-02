import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import {stripTypeScriptTypes} from 'node:module'
import {test} from 'node:test'
import vm from 'node:vm'

const source=stripTypeScriptTypes(await readFile(new URL('../src/option-definitions-api.ts',import.meta.url),'utf8'))
async function fixture(){
 const state={response:undefined,csrf:'a'.repeat(64),calls:[]}
 const context=vm.createContext({state,URLSearchParams})
 const api=new vm.SourceTextModule(`export class APIError extends Error {constructor(status,code){super(code);this.status=status;this.code=code}};export function csrfCookie(){return state.csrf};export async function request(path,options,base){state.calls.push({path,options,base});if(state.response instanceof Error)throw state.response;return state.response}`,{context})
 const mod=new vm.SourceTextModule(source,{context});await mod.link(spec=>{assert.equal(spec,'./api');return api});await mod.evaluate()
 return {state,api:mod.namespace}
}
const revision='a'.repeat(64),operation='d89473cd-86ce-4240-a1c7-d3c9842ea421'
const row=()=>({id:'opt_a',title:'容量',metadata_json:null,values:[{id:'value_a',value:'128 GB',rank:null,memberships:1}],active_products:1,is_exclusive:false,deleted:false,enabled:true,revision})
const page=()=>({options:[row()],count:1,offset:0,limit:20,enabled:true})

test('shared definition reads reject contradictory identities and incomplete pages',async()=>{
 const {state,api}=await fixture();state.response=page();assert.equal((await api.optionDefinitions()).options[0].values[0].memberships,1)
 for(const mutate of [d=>{d.options[0].id='bad/id'},d=>{d.count=2},d=>{d.options[0].enabled=false},d=>{d.options[0].deleted="yes"},d=>{d.options[0].values.push({...d.options[0].values[0]})},d=>{d.options[0].values[0].rank=2147483648},d=>{d.options[0].values[0].memberships=-1}]){
  const d=page();mutate(d);state.response=d;await assert.rejects(api.optionDefinitions(),e=>e.code==='request.invalid_response')
 }
 state.response=row();await assert.rejects(api.optionDefinition('opt_other'),e=>e.code==='request.invalid_response')
 const before=state.calls.length;await assert.rejects(api.optionDefinitions(-1));await assert.rejects(api.optionDefinition('../secret'));assert.equal(state.calls.length,before)
})

test('commands require current CSRF and send the original immutable operation',async()=>{
 const {state,api}=await fixture();const input=Object.freeze({operation_id:operation,expected_revision:revision,title:'容量',metadata_json:null,values:Object.freeze(['128 GB']),ranks:Object.freeze({'128 GB':0})})
 state.csrf='';await assert.rejects(api.saveOptionDefinition('opt_a',input),e=>e.status===403);assert.equal(state.calls.length,0)
 state.csrf='b'.repeat(64);state.response={id:'opt_a',revision,replayed:false};await api.saveOptionDefinition('opt_a',input)
 state.response={id:'opt_a',revision,replayed:true};await api.saveOptionDefinition('opt_a',input)
 assert.equal(state.calls.length,2);for(const call of state.calls){assert.equal(call.path,'option-definitions/opt_a');assert.equal(call.base,'/api/admin/');assert.equal(call.options.csrf,state.csrf);assert.equal(call.options.body,input)}
 state.response={id:'opt_foreign',revision,replayed:true};await assert.rejects(api.saveOptionDefinition('opt_a',input),e=>e.code==='request.invalid_response')
})

test('value removal remains scoped to its definition and canonical operation',async()=>{
 const {state,api}=await fixture();state.response={id:'opt_a',revision,replayed:false};const input={operation_id:operation,expected_revision:revision}
 await api.removeOptionDefinition('opt_a','value_a',input);assert.equal(state.calls[0].path,'option-definitions/opt_a/values/value_a/remove')
 await api.removeOptionDefinition('opt_a',undefined,input);assert.equal(state.calls[1].path,'option-definitions/opt_a/remove')
 const before=state.calls.length;await assert.rejects(async()=>api.removeOptionDefinition('opt_a','../foreign',input));await assert.rejects(async()=>api.removeOptionDefinition('opt_a',undefined,{...input,operation_id:operation.toUpperCase()}));assert.equal(state.calls.length,before)
})


test('definition filters encode literal values and reject invalid transport',async()=>{
 const {state,api}=await fixture();state.response=page()
 await api.optionDefinitions(0,undefined,{q:'容量%_! &',title:'Exact +',id:'opt_a',sort:'updated_at_desc'})
 const params=new URLSearchParams(state.calls[0].path.split('?')[1]);assert.equal(params.get('q'),'容量%_! &');assert.equal(params.get('title'),'Exact +');assert.equal(params.get('sort'),'updated_at_desc')
 const before=state.calls.length
 for(const filters of [{q:' '},{q:'x'.repeat(201)},{id:'../x'},{sort:'DROP TABLE'},{title:'x'.repeat(101)},{q:'a\n'},{unknown:'x'}])await assert.rejects(api.optionDefinitions(0,undefined,filters))
 assert.equal(state.calls.length,before)
})


test('metadata exact JSON reads and canonical scoped commands',async()=>{
 const {state,api}=await fixture();state.response={...row(),metadata_json:'{"large":9007199254740993123}'}
 assert.equal((await api.optionDefinition('opt_a')).metadata_json,'{"large":9007199254740993123}')
 for(const value of [undefined,{},'[]','null','{']){state.response={...row(),metadata_json:value};await assert.rejects(api.optionDefinition('opt_a'))}
 state.response={id:'opt_a',revision,replayed:false}
 const input={operation_id:operation,expected_revision:revision,metadata:{remove:'',enabled:false}}
 await api.saveDefinitionMetadata('opt_a',input);assert.equal(state.calls.at(-1).path,'option-definitions/opt_a/metadata');assert.equal(state.calls.at(-1).options.body,input)
 await api.saveDefinitionMetadata('opt_a',{...input,metadata:null})
 const before=state.calls.length
 await assert.rejects(async()=>api.saveDefinitionMetadata('../other',input));await assert.rejects(async()=>api.saveDefinitionMetadata('opt_a',{...input,metadata:[]}));assert.equal(state.calls.length,before)
})


test('multi-value and time filters preserve source values and reject ambiguous shapes',async()=>{
 const {state,api}=await fixture();state.response=page()
 await api.optionDefinitions(20,undefined,{ids:['a','b'],titles:['One, two','中文'],'created_at[$gte]':'2026-01-02T08:00:00.123456+08:00','updated_at[$lt]':'2026-02-01T00:00:00Z'}).catch(e=>{assert.equal(e.code,'request.invalid_response')})
 const params=new URLSearchParams(state.calls[0].path.split('?')[1]);assert.deepEqual(params.getAll('id[]'),['a','b']);assert.deepEqual(params.getAll('title[]'),['One, two','中文']);assert.equal(params.get('created_at[$gte]'),'2026-01-02T08:00:00.123456+08:00')
 const before=state.calls.length
 for(const filters of [{ids:[]},{ids:['a','a']},{ids:['a'],id:'a'},{titles:['x'],title:'x'},{ids:['../x']},{titles:['']},{'created_at[$like]':'2026-01-01T00:00:00Z'},{'updated_at[$gte]':'2026-01-01'},{'updated_at[$gte]':'not-a-date'},{'updated_at[$gte]':'2026-01-01T00:00:00.1234567Z'}])await assert.rejects(api.optionDefinitions(0,undefined,filters))
 assert.equal(state.calls.length,before)
})

test('scope, deleted dates and compound URLs preserve source leaf values',async()=>{
 const {state,api}=await fixture();state.response=page()
 state.response.options[0].is_exclusive=true;state.response.options[0].deleted=true
 const filters={is_exclusive:'true',with_deleted:'true','deleted_at[$gte]':'2026-01-01T00:00:00Z','$and[0][$or][0][title]':'One, two','$and[0][$or][1][id][]':['a','b']}
 const result=await api.optionDefinitions(0,undefined,filters);assert.equal(result.options[0].deleted,true);assert.equal(result.options[0].is_exclusive,true)
 const p=new URLSearchParams(state.calls[0].path.split('?')[1]);assert.equal(p.get('is_exclusive'),'true');assert.deepEqual(p.getAll('$and[0][$or][1][id][]'),['a','b'])
 const before=state.calls.length
 for(const bad of [{is_exclusive:'1'},{with_deleted:'yes'},{'$or[16][id]':'a'},{'$or[0][offset]':'20'},{'$or[0][id]':['a','b']},{'$and[0][id]':'../x'}])await assert.rejects(api.optionDefinitions(0,undefined,bad))
 assert.equal(state.calls.length,before)
})
