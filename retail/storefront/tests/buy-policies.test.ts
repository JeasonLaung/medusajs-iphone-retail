import test from 'node:test'
import assert from 'node:assert/strict'
import {loadBuyPolicies} from '../src/lib/buy-policies.ts'
test('buy policies retain explicit zero, trim translated notes, omit private fields and distinguish dependency failure',async()=>{
 const policies={warranty_months:0,return_days:0,warranty_note:{en:' Note ',zh:'说明',private:'hidden'},return_note:{en:'',zh:''},private:'hidden'}
 const got=await loadBuyPolicies('https://gin.example',async(input,init)=>{assert.equal(String(input),'https://gin.example/api/store/buy-policies');assert.equal(init?.credentials,'omit');assert.equal(init?.redirect,'error');return Response.json({policies,private:'hidden'})})
 assert.deepEqual(got,{warranty_months:0,return_days:0,warranty_note:{en:'Note',zh:'说明'},return_note:{en:'',zh:''}})
 for(const patch of [{warranty_months:1.5},{return_days:366},{warranty_months:'24'},{warranty_note:{en:'😀'.repeat(101),zh:''}}])assert.equal(await loadBuyPolicies('https://gin.example',async()=>Response.json({policies:{...policies,...patch}})),null)
 assert.equal(await loadBuyPolicies(undefined),null)
 assert.equal(await loadBuyPolicies('https://gin.example',async()=>Response.json({policies},{status:503})),null)
})
