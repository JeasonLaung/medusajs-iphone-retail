import test from 'node:test'
import assert from 'node:assert/strict'
import {reviewID,reviewQuery,productReviewPage,reviewPurchases,orderReviewContext,reviewReceipt,loadProductReviews} from '../src/lib/product-reviews.ts'
import {customerClient,CustomerError} from '../src/lib/customer-client.ts'
import {customerBridge} from '../src/lib/customer-bridge.ts'

const product='prod_fox_17',operation='00000000-0000-4000-8000-000000000001',stamp='2026-10-31T00:00:00Z'
const review={id:'review_fox_1',author:'Customer',rating:4,title:'A good phone',content:'The phone works well.',created_at:stamp,verified_purchase:false}
const page=()=>({reviews:[{...review,account_id:'PRIVATE',order_id:'PRIVATE',status:'approved'}],count:1,offset:0,limit:5,average:4,distribution:[1,2,3,4,5].map(rating=>({rating,count:rating===4?1:0})),internal:'PRIVATE'})
const purchases=()=>({orders:[{id:'order_1',number:'1001',created_at:stamp,eligible:true,email:'PRIVATE'}],review:null,count:1,offset:0,limit:20})
const shop='http://127.0.0.1:9300',root='/api/store/customer',config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:shop},csrf='a'.repeat(64)
const privateResponse=(body:unknown,status=200)=>Response.json(body,{status,headers:{'Cache-Control':'private, no-store'}})
const get=(path:string)=>new Request(shop+root+path)
const post=()=>new Request(shop+root+'/product-reviews',{method:'POST',headers:{Origin:shop,'Content-Type':'application/json','X-CSRF-Token':csrf,Cookie:`retail_customer_session=${csrf}; retail_admin_session=PRIVATE`},body:JSON.stringify({operation_id:operation,product_id:product,rating:4,title:review.title,content:review.content})})

test('review IDs and queries retain valid source IDs and reject path/control/ambiguous query inputs',()=>{
 for(const id of [product,'review_fox_1','机型😀','😀'.repeat(255)])assert.equal(reviewID(id),true)
 for(const id of ['',null,{},'a\x00b','a\nb','a/b','a\\b','a?b','a#b','😀'.repeat(256)])assert.equal(reviewID(id),false)
 assert.deepEqual(reviewQuery(new URLSearchParams({product_id:product})),{productID:product,offset:0,limit:5})
 for(const query of ['','product_id=x&product_id=y','product_id=x&offset=01','product_id=x&offset=-1','product_id=x&offset=100001','product_id=x&limit=0','product_id=x&limit=21','product_id=x&customer_id=PRIVATE'])assert.throws(()=>reviewQuery(new URLSearchParams(query)))
})

test('public review projection excludes customer/order/status fields and verifies aggregate/page consistency',()=>{
 const data=productReviewPage(page())
 assert.deepEqual(data.reviews,[review]);assert.ok(!JSON.stringify(data).includes('PRIVATE'))
 const broken=[{...page(),average:4.1},{...page(),count:2},{...page(),reviews:[]},{...page(),offset:5},{...page(),limit:20},{...page(),reviews:[{...review,rating:0}]},{...page(),reviews:[{...review,verified_purchase:'yes'}]},{...page(),reviews:[{...review,created_at:'invalid'}]},{...page(),distribution:[{rating:4,count:1}]}]
 for(const raw of broken)assert.throws(()=>productReviewPage(raw))
 const duplicate={...page(),count:2,reviews:[review,review],distribution:[1,2,3,4,5].map(rating=>({rating,count:rating===4?2:0}))}
 assert.throws(()=>productReviewPage(duplicate))
 assert.deepEqual(productReviewPage({...page(),reviews:[],offset:5},5).reviews,[])
})

test('public fetch uses a fixed credential-free endpoint and rejects unavailable or malformed success',async()=>{
 const result=await loadProductReviews(config.ginOrigin,product,0,5,async(url,init)=>{
  assert.equal(String(url),config.ginOrigin+'/api/store/product-reviews?product_id='+product+'&offset=0&limit=5')
  assert.equal(init?.credentials,'omit');assert.equal(init?.cache,'no-store');assert.equal(init?.redirect,'error')
  assert.deepEqual([...new Headers(init?.headers)],[['accept','application/json']])
  return Response.json(page())
 })
 assert.deepEqual(result.reviews,[review])
 for(const response of [Response.json(page(),{status:201}),Response.json(page(),{status:503}),Response.json({...page(),average:5}),new Response('not json')])await assert.rejects(loadProductReviews(config.ginOrigin,product,0,5,async()=>response))
})

test('private review projections retain only eligibility and current review status',()=>{
 assert.ok(!JSON.stringify(reviewPurchases(purchases())).includes('PRIVATE'))
 assert.deepEqual(orderReviewContext({eligible:true,reviews:[{product_id:product,status:'pending',verified_purchase:true,account_id:'PRIVATE'}]}),{eligible:true,reviews:[{product_id:product,status:'pending',verified_purchase:true}]})
 assert.throws(()=>reviewPurchases({...purchases(),limit:5}))
 assert.throws(()=>reviewPurchases({...purchases(),review:{id:'r',status:'unknown',verified_purchase:false}}))
 assert.throws(()=>orderReviewContext({eligible:true,reviews:[{product_id:product,status:'pending',verified_purchase:true},{product_id:product,status:'approved',verified_purchase:true}]}))
 assert.throws(()=>reviewReceipt({review:{id:'r',status:'pending',replayed:'yes'}}))
})

test('customer review bridge admits only explicit private read/write routes and canonical purchase queries',async()=>{
 let calls=0
 for(const path of ['/product-reviews/purchases?product_id='+product,'/product-reviews/purchases?offset=20&product_id='+product,'/orders/order_1/product-reviews']){
  const result=await customerBridge(get(path),config,async(url)=>{calls++;assert.equal(String(url),config.ginOrigin+root+path);return privateResponse({eligible:true})})
  assert.equal(result.status,200);assert.equal(result.headers.get('Cache-Control'),'private, no-store')
 }
 for(const path of ['/product-reviews','/product-reviews/purchases','/product-reviews/purchases?product_id='+product+'&limit=5','/product-reviews/purchases?product_id=x&product_id=y','/product-reviews/purchases?product_id=x&offset=01','/orders/order_1/product-reviews?customer_id=PRIVATE'])assert.equal((await customerBridge(get(path),config,async()=>{calls++;return privateResponse({})})).status,404)
 assert.equal(calls,3)
 const result=await customerBridge(post(),config,async(url,init)=>{
  assert.equal(String(url),config.ginOrigin+root+'/product-reviews');assert.equal(init?.method,'POST')
  const headers=new Headers(init?.headers);assert.equal(headers.get('Cookie'),`retail_customer_session=${csrf}`);assert.equal(headers.get('X-CSRF-Token'),csrf);assert.equal(headers.get('Authorization'),null)
  return privateResponse({review:{id:review.id,status:'pending',replayed:false}},201)
 })
 assert.equal(result.status,201);assert.equal(result.headers.getSetCookie().length,0)
})

test('review bridge rejects incorrect success statuses, cookie issuance, and unapproved error details',async()=>{
 for(const [request,status]of [[post(),200],[post(),202],[get('/orders/order_1/product-reviews'),201],[get('/orders/order_1/product-reviews'),202]] as const)assert.equal((await customerBridge(request,config,async()=>privateResponse({},status))).status,503)
 const cookieResult=await customerBridge(post(),config,async()=>{const response=privateResponse({},201);response.headers.set('Set-Cookie',`retail_customer_session=${csrf}; Path=/; HttpOnly; SameSite=Lax`);return response})
 assert.equal(cookieResult.status,503);assert.equal(cookieResult.headers.getSetCookie().length,0)
 for(const [code,status]of [['review.already_submitted',409],['review.order_ineligible',403],['review.conflict',409],['review.invalid_request',400]] as const){
  const response=await customerBridge(post(),config,async()=>privateResponse({error:{code,message:'PRIVATE_DATABASE',sql:'PRIVATE'}},status))
  assert.equal(response.status,status);const body=await response.json();assert.equal(body.error.code,code);assert.ok(!JSON.stringify(body).includes('PRIVATE'))
 }
 const mismatched=await customerBridge(post(),config,async()=>privateResponse({error:{code:'review.order_ineligible',message:'PRIVATE'}},409))
 assert.ok(!(await mismatched.text()).includes('review.order_ineligible'))
})

test('customer submission preserves immutable operation on explicit retry and binds current session CSRF',async()=>{
 const priorDocument=Object.getOwnPropertyDescriptor(globalThis,'document'),priorLocation=Object.getOwnPropertyDescriptor(globalThis,'location')
 Object.defineProperty(globalThis,'document',{value:{cookie:`retail_customer_csrf=${csrf}`},configurable:true});Object.defineProperty(globalThis,'location',{value:{protocol:'http:'},configurable:true})
 let calls=0
 const input={operation_id:operation,product_id:product,rating:4,title:review.title,content:review.content}
 try{
  const api=customerClient(async(url,init)=>{
   calls++;assert.equal(String(url),root+'/product-reviews');assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),csrf);assert.deepEqual(JSON.parse(String(init?.body)),input)
   if(calls===1)throw new Error('Response lost after commit')
   return Response.json({review:{id:review.id,status:'pending',replayed:true}},{status:201})
  })
  await assert.rejects(api.submitReview(input));assert.deepEqual(await api.submitReview(input),{id:review.id,status:'pending',replayed:true});assert.equal(calls,2)
  for(const bad of [{...input,rating:0},{...input,operation_id:'bad'},{...input,title:'x'},{...input,order_id:'../foreign'}])await assert.rejects(api.submitReview(bad),CustomerError)
  assert.equal(calls,2)
  await assert.rejects(customerClient(async()=>Response.json({review:{id:'r',status:'pending',replayed:false}})).submitReview(input),CustomerError)
 }finally{
  if(priorDocument)Object.defineProperty(globalThis,'document',priorDocument);else Reflect.deleteProperty(globalThis,'document')
  if(priorLocation)Object.defineProperty(globalThis,'location',priorLocation);else Reflect.deleteProperty(globalThis,'location')
 }
})
