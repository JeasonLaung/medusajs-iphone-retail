import {APIError,csrfCookie,request} from './api'

export type DefinitionValue={id:string;value:string;rank:number|null;memberships:number}
export type OptionDefinition={id:string;title:string;metadata_json:string|null;values:DefinitionValue[];active_products:number;is_exclusive:boolean;deleted:boolean;enabled:boolean;revision:string}
export type OptionDefinitionPage={options:OptionDefinition[];count:number;offset:number;limit:20;enabled:boolean}
export type DefinitionInput={operation_id:string;expected_revision:string;title:string;values:string[];ranks?:Record<string,number>}
export type DefinitionRemoval={operation_id:string;expected_revision:string}
export type DefinitionResult={id:string;revision:string;replayed:boolean}
const invalid=()=>new APIError(0,'request.invalid_response')
const id=(v:unknown):v is string=>typeof v==='string'&&v.length>0&&[...v].length<=191&&!/[\x00-\x1f\x7f/\\?#]/.test(v)
const revision=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v)
const count=(v:unknown):v is number=>Number.isSafeInteger(v)&&Number(v)>=0
const rank=(v:unknown):v is number=>Number.isInteger(v)&&Number(v)>=-2147483648&&Number(v)<=2147483647
function definition(d:OptionDefinition):OptionDefinition{
 if(!d||!id(d.id)||typeof d.title!=='string'||!revision(d.revision)||typeof d.enabled!=='boolean'||typeof d.deleted!=='boolean'||typeof d.is_exclusive!=='boolean'||!count(d.active_products)||!Array.isArray(d.values)||d.values.length>100)throw invalid()
 if(d.metadata_json!==null){
  if(typeof d.metadata_json!=='string')throw invalid()
  let metadata:unknown;try{metadata=JSON.parse(d.metadata_json)}catch{throw invalid()}
  if(!metadata||typeof metadata!=='object'||Array.isArray(metadata))throw invalid()
 }
 const values=d.values.map(v=>{
  if(!v||!id(v.id)||typeof v.value!=='string'||v.rank!==null&&!rank(v.rank)||!count(v.memberships))throw invalid()
  return {id:v.id,value:v.value,rank:v.rank,memberships:v.memberships}
 })
 if(new Set(values.map(v=>v.id)).size!==values.length||new Set(values.map(v=>v.value)).size!==values.length)throw invalid()
 return {id:d.id,title:d.title,metadata_json:d.metadata_json,revision:d.revision,enabled:d.enabled,deleted:d.deleted,is_exclusive:d.is_exclusive,active_products:d.active_products,values}
}
export type DefinitionDateKey=`${'created_at'|'updated_at'|'deleted_at'}[$${'gt'|'gte'|'lt'|'lte'|'eq'|'ne'}]`
export type DefinitionFilters={q?:string;title?:string;id?:string;sort?:string;is_exclusive?:string;with_deleted?:string;ids?:string[];titles?:string[]}&Partial<Record<DefinitionDateKey,string>>&Partial<Record<`$${'and'|'or'}[${number}][${string}]${string}`,string|string[]>>
export function definitionFilterParams(offset:number,filters:DefinitionFilters):URLSearchParams{
 if(!count(offset)||offset>100000)throw new APIError(400,'catalog.invalid_option_definition')
 const params=new URLSearchParams({offset:String(offset)}),bad=()=>new APIError(400,'catalog.invalid_option_definition')
 for(const [key,value] of Object.entries(filters)){
  if(value===undefined)continue
  if(key.startsWith('$')){
   let leaf=key,depth=0
   while(leaf.startsWith('$')){
    const match=/^\$(and|or)\[(0|[1-9][0-9]*)\]\[([^\]]+)\](\[.*\])?$/.exec(leaf)
    if(!match||Number(match[2])>15||++depth>4)throw bad()
    leaf=match[3]+(match[4]??'')
   }
   if(!['q','title','id','is_exclusive','id[]','title[]'].includes(leaf)&&!/^(created_at|updated_at|deleted_at)\[\$(gt|gte|lt|lte|eq|ne)\]$/.test(leaf))throw bad()
   const array=leaf==='id[]'||leaf==='title[]'
   const name=array?(leaf==='id[]'?'ids':'titles'):leaf
   const values=Array.isArray(value)?value:[value]
   if(!array&&values.length!==1)throw bad()
   definitionFilterParams(0,{[name]:array?values:values[0]})
   for(const entry of values)params.append(key,entry)
   continue
  }
  if(key==='ids'||key==='titles'){
   if(!Array.isArray(value)||value.length<1||value.length>100||new Set(value).size!==value.length||filters[key==='ids'?'id':'title']!==undefined)throw bad()
   for(const entry of value){
    if(typeof entry!=='string'||!entry||/[\x00-\x1f\x7f]/.test(entry)||key==='ids'&&!id(entry)||key==='titles'&&[...entry].length>100)throw bad()
    params.append(key==='ids'?'id[]':'title[]',entry)
   }
   continue
  }
  if(typeof value!=='string'||!value||/[\x00-\x1f\x7f]/.test(value))throw bad()
  if(key==='is_exclusive'||key==='with_deleted'){if(value!=='true'&&value!=='false')throw bad()}
  else if(['q','title','id','sort'].includes(key)){
   if(key==='q'&&(!value.trim()||[...value].length>200)||key==='title'&&[...value].length>100||key==='id'&&!id(value)||key==='sort'&&!['title_asc','title_desc','created_at_asc','created_at_desc','updated_at_asc','updated_at_desc'].includes(value))throw bad()
  }else{
   if(!/^(created_at|updated_at|deleted_at)\[\$(gt|gte|lt|lte|eq|ne)\]$/.test(key)||!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,6})?(Z|[+-]\d{2}:\d{2})$/.test(value)||!Number.isFinite(Date.parse(value)))throw bad()
  }
  params.set(key,value)
 }
 return params
}
export async function optionDefinitions(offset=0,signal?:AbortSignal,filters:DefinitionFilters={}):Promise<OptionDefinitionPage>{
 const params=definitionFilterParams(offset,filters)
 const d=await request<OptionDefinitionPage>('option-definitions?'+params,{signal},'/api/admin/')
 if(!d||!count(d.count)||d.offset!==offset||d.limit!==20||typeof d.enabled!=='boolean'||!Array.isArray(d.options)||d.options.length!==Math.min(20,Math.max(0,d.count-offset)))throw invalid()
 const options=d.options.map(definition)
 if(new Set(options.map(o=>o.id)).size!==options.length||options.some(o=>o.enabled!==d.enabled))throw invalid()
 return {options,count:d.count,offset,limit:20,enabled:d.enabled}
}
export async function optionDefinition(idValue:string,signal?:AbortSignal):Promise<OptionDefinition>{
 if(!id(idValue))throw new APIError(400,'catalog.invalid_option_definition')
 const d=definition(await request<OptionDefinition>('option-definitions/'+encodeURIComponent(idValue),{signal},'/api/admin/'))
 if(d.id!==idValue)throw invalid()
 return d
}
async function command(path:string,input:DefinitionInput|DefinitionRemoval|DefinitionMetadataInput,expectedID?:string):Promise<DefinitionResult>{
 if(!revision(input.expected_revision)||!/^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/.test(input.operation_id))throw new APIError(400,'catalog.invalid_option_definition')
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<DefinitionResult>(path,{method:'POST',body:input,csrf},'/api/admin/')
 if(!d||!id(d.id)||expectedID!==undefined&&d.id!==expectedID||!revision(d.revision)||typeof d.replayed!=='boolean')throw invalid()
 return {id:d.id,revision:d.revision,replayed:d.replayed}
}
export function saveOptionDefinition(idValue:string|undefined,input:DefinitionInput):Promise<DefinitionResult>{
 if(idValue!==undefined&&!id(idValue))throw new APIError(400,'catalog.invalid_option_definition')
 return command('option-definitions'+(idValue?'/'+encodeURIComponent(idValue):''),input,idValue)
}
export function removeOptionDefinition(idValue:string,valueID:string|undefined,input:DefinitionRemoval):Promise<DefinitionResult>{
 if(!id(idValue)||valueID!==undefined&&!id(valueID))throw new APIError(400,'catalog.invalid_option_definition')
 return command('option-definitions/'+encodeURIComponent(idValue)+(valueID?'/values/'+encodeURIComponent(valueID):'')+'/remove',input,idValue)
}

export type DefinitionMetadataInput=DefinitionRemoval&{metadata:Record<string,unknown>|null}
export function saveDefinitionMetadata(idValue:string,input:DefinitionMetadataInput):Promise<DefinitionResult>{
 if(!id(idValue)||input.metadata!==null&&(!input.metadata||typeof input.metadata!=='object'||Array.isArray(input.metadata)))throw new APIError(400,'catalog.invalid_option_definition')
 return command('option-definitions/'+encodeURIComponent(idValue)+'/metadata',input,idValue)
}
