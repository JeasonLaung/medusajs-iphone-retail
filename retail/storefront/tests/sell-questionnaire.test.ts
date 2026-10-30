import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import {createRequire} from 'node:module'
import vm from 'node:vm'
import {restoreAnswers,emptyAnswers,validQuestionnaireRoute,questionnaireStages,questionnaireGuard,answersForRoute,questionnairePath,stageComplete} from '../src/lib/sell-questionnaire.ts'
const require=createRequire(import.meta.url),ts=require('typescript')
const models=[{id:'p13',name:'iPhone 13',handle:'iphone-13',capacities:['128GB','256GB']},{id:'p14',name:'iPhone 14',handle:'iphone-14',capacities:['1TB']}]
const healthy={model:'iphone-13',memory:'128GB',battery:'90',checks:[true,true,true,true,true],faults:['none'],glass:['none'],screen:'none',frame:'none',back:'none'}
async function original(){
 const source=await readFile(new URL('../../../medusa-store/apps/storefront/src/modules/sell/components/sell-flow.tsx',import.meta.url),'utf8')
 const exports:Record<string,Function>={}
 const compiled=ts.transpileModule(source+'\nexport {restore};',{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.CommonJS}}).outputText
 vm.runInNewContext(compiled,{exports,require:()=>({})})
 return (raw:string|null)=>JSON.parse(JSON.stringify(exports.restore(raw,models)))
}
test('session restoration matches actual original questionnaire function for valid and malformed stored answers',async()=>{
 const restore=await original(),cases:(string|null)[]=[null,'','{','null','[]','1','{}',JSON.stringify(healthy)]
 const patch=(v:Record<string,unknown>)=>cases.push(JSON.stringify({...healthy,...v}))
 for(const model of ['unknown','',null,13])patch({model})
 for(const memory of ['128 GB','128gb','1TB','',null])patch({memory})
 for(const battery of ['','unknown','001','101','0',90,null,'９０'])patch({battery})
 for(const checks of [[],[true],[true,false,null,0,'true',false],null,{},'bad'])patch({checks})
 for(const field of ['faults','glass'])for(const values of [[],['none'],['none','pixels'],['pixels','pixels'],['cracked'],null,{},[false]])patch({[field]:values})
 for(const field of ['screen','frame','back'])for(const value of ['damaged','none','invalid',null])patch({[field]:value})
 for(const raw of cases)assert.deepEqual(restoreAnswers(raw,models),restore(raw),'stored questionnaire differs from original')
})
test('questionnaire routes preserve original model and capacity URL validation',()=>{
 for(const steps of [['model'],['iphone-13','memory'],['iphone-13','128gb','battery'],['iphone-14','1tb','summary']])assert.equal(validQuestionnaireRoute(steps,models),true)
 for(const steps of [[],['summary'],['iphone-13'],['iphone-13','128GB','battery'],['iphone-13','1tb','summary'],['iphone-13','128gb','bad'],['iphone-13','128gb','battery','extra']])assert.equal(validQuestionnaireRoute(steps,models),false)
 assert.equal(questionnairePath('dk-en','memory',healthy),'/dk-en/sell/iphone/iphone-13/memory')
 assert.equal(questionnairePath('dk-zh','battery',healthy),'/dk-zh/sell/iphone/iphone-13/128gb/battery')
})
test('changing URL phone resets dependent answers while ordinary step navigation preserves them',()=>{
 assert.deepEqual(answersForRoute(healthy,['iphone-13','128gb','screen'],models),healthy)
 assert.deepEqual(answersForRoute(healthy,['iphone-13','256gb','battery'],models),{...emptyAnswers(),model:'iphone-13',memory:'256GB'})
 assert.deepEqual(answersForRoute(healthy,['iphone-14','memory'],models),{...emptyAnswers(),model:'iphone-14'})
 assert.deepEqual(answersForRoute(healthy,['iphone-14','1tb','summary'],models),{...emptyAnswers(),model:'iphone-14',memory:'1TB'})
})
test('guards preserve first unanswered question and broken screen/frame skips',()=>{
 const a={...healthy,faults:['pixels'],screen:'',frame:'damaged',back:''}
 assert.deepEqual(questionnaireStages(a),['model','memory','battery','functionality','screen-faults','screen-glass','frame'])
 assert.equal(questionnaireGuard('dk-en','summary',a),'')
 assert.equal(questionnaireGuard('dk-en','screen',a),'/dk-en/sell/iphone/iphone-13/128gb/frame')
 assert.equal(questionnaireGuard('dk-en','back',a),'/dk-en/sell/iphone/iphone-13/128gb/summary')
 for(const [field,value,expected]of [['battery','','battery'],['checks',[true,true,null,true,true],'functionality'],['faults',[],'screen-faults'],['glass',['none','cracked'],'screen-glass'],['screen','','screen'],['frame','','frame'],['back','','back']] as const){
  const incomplete={...healthy,[field]:value}
  assert.equal(questionnaireGuard('dk-en','summary',incomplete),'/dk-en/sell/iphone/iphone-13/128gb/'+expected)
 }
 assert.equal(stageComplete('functionality',{...healthy,checks:[false,true,true,true,true]}),false)
 assert.equal(questionnaireGuard('dk-en','model',emptyAnswers()),'')
})
