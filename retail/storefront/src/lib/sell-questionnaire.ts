import type {SellModel} from './sell-models.ts'
import type {BuybackAssessment} from './buyback-client.ts'

export type SellAnswers=BuybackAssessment&{model:string;memory:string}
export const questionnaireKey='sell-iphone-questionnaire-v2'
export const baseStages=['model','memory','battery','functionality','screen-faults','screen-glass','screen','frame','back']
export const gradeIDs=['damaged','visible','some','minimal','none']
export const faultIDs=['bright','pixels','lines']
export const glassIDs=['cracked','chipped','scratched']
export const emptyAnswers=():SellAnswers=>({model:'',memory:'',battery:'',checks:[null,null,null,null,null],faults:[],glass:[],screen:'',frame:'',back:''})
export const validBattery=(s:string)=>s==='unknown'||/^\d{1,3}$/.test(s)&&Number(s)>=1&&Number(s)<=100
export const validMulti=(values:string[],allowed:string[])=>values.length>0&&values.every(v=>allowed.includes(v))&&(!values.includes('none')||values.length===1)

// Preserve the original partial restore order: an invalid capacity still keeps
// the selected model, while answers after that capacity remain unset.
export function restoreAnswers(raw:string|null,models:SellModel[]):SellAnswers{
 const result=emptyAnswers()
 if(!raw)return result
 try{
  const value=JSON.parse(raw),model=models.find(m=>m.handle===value.model)
  if(!model)return result
  result.model=model.handle
  if(!model.capacities.includes(value.memory))return result
  result.memory=value.memory
  if(typeof value.battery==='string'&&validBattery(value.battery))result.battery=value.battery
  if(Array.isArray(value.checks))result.checks=result.checks.map((_,i)=>typeof value.checks[i]==='boolean'?value.checks[i]:null)
  for(const [field,allowed]of [['faults',[...faultIDs,'none']],['glass',[...glassIDs,'none']]] as const){
   if(Array.isArray(value[field])&&validMulti(value[field],[...allowed]))result[field]=value[field]
  }
  for(const field of ['screen','frame','back'] as const)if(gradeIDs.includes(value[field]))result[field]=value[field]
 }catch{}
 return result
}
export function questionnaireStages(a:SellAnswers){
 const skip=a.faults.some(v=>v!=='none')||a.glass.some(v=>v!=='none')
 return baseStages.filter(s=>!(s==='screen'&&skip)&&!(s==='back'&&a.frame==='damaged'))
}
export function stageComplete(stage:string,a:SellAnswers){
 if(stage==='model')return Boolean(a.model)
 if(stage==='memory')return Boolean(a.memory)
 if(stage==='battery')return validBattery(a.battery)
 if(stage==='functionality')return a.checks[0]!==false&&a.checks[1]!==false&&a.checks.every(v=>v!==null)
 if(stage==='screen-faults')return validMulti(a.faults,[...faultIDs,'none'])
 if(stage==='screen-glass')return validMulti(a.glass,[...glassIDs,'none'])
 return gradeIDs.includes(a[stage as 'screen'|'frame'|'back'])
}
export function validQuestionnaireRoute(steps:string[],models:SellModel[]){
 return steps.length===1&&steps[0]==='model'||steps.length===2&&models.some(m=>m.handle===steps[0])&&steps[1]==='memory'||steps.length===3&&models.some(m=>m.handle===steps[0]&&m.capacities.some(c=>c.toLowerCase()===steps[1]))&&['battery','functionality','screen-faults','screen-glass','screen','frame','back','summary'].includes(steps[2])
}
export function questionnairePath(segment:string,stage:string,a:SellAnswers){
 const root=`/${segment}/sell/iphone`
 if(stage==='model')return `${root}/model`
 if(stage==='memory')return `${root}/${a.model}/memory`
 return `${root}/${a.model}/${a.memory.toLowerCase()}/${stage}`
}

// URL model/capacity changes discard answers from the previous phone exactly as
// the original route effect does. URL validity is checked before this function.
export function answersForRoute(a:SellAnswers,steps:string[],models:SellModel[]):SellAnswers{
 const model=models.find(m=>m.handle===steps[0]),memory=steps.length===3?steps[1].toUpperCase():''
 if(model&&model.handle!==a.model)a={...emptyAnswers(),model:model.handle}
 if(memory&&a.memory!==memory)a={...emptyAnswers(),model:a.model,memory}
 return a
}
export function questionnaireGuard(segment:string,stage:string,a:SellAnswers){
 if(stage==='model'||stage==='memory')return ''
 const stages=questionnaireStages(a),previous=stages.slice(0,stage==='summary'?stages.length:stages.indexOf(stage))
 const missing=previous.find(s=>!stageComplete(s,a))
 if(missing)return questionnairePath(segment,missing,a)
 if(!stages.includes(stage)&&stage!=='summary')return questionnairePath(segment,stage==='screen'?'frame':'summary',a)
 return ''
}
