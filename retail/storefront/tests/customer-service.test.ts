import test from 'node:test'
import assert from 'node:assert/strict'
import {loadServiceContacts} from '../src/lib/customer-service.ts'
import {isWithinServiceHours,whatsappLink} from '../src/components/customer-service/contact.ts'

test('public service contacts project only published fields and retain schedule boundaries and safe channels',async()=>{
 const contact={id:'support',name:'Support',department:'after_sales',whatsapp:'+4512345678',email:'help@example.test',timezone:'Europe/Copenhagen',days:[1,2,3,4,5],start:'09:00',end:'17:00',availability:'schedule'}
 const loaded=await loadServiceContacts('https://gin.example',async(input,init)=>{
  assert.equal(String(input),'https://gin.example/api/store/customer-service');assert.equal(init?.credentials,'omit')
  return Response.json({contacts:[{...contact,private:'hidden',enabled:true}]})
 })
 assert.deepEqual(loaded,[contact])
 assert.equal(whatsappLink(contact.whatsapp),'https://wa.me/4512345678');assert.equal(whatsappLink('https://evil.test'),null)
 assert.equal(isWithinServiceHours(loaded[0],new Date('2026-10-26T08:00:00Z')),true)
 assert.equal(isWithinServiceHours(loaded[0],new Date('2026-10-26T16:00:00Z')),false)
 for(const edit of [{whatsapp:'javascript:alert(1)'},{email:'x@example.test\r\nBcc:secret'},{timezone:'unknown-zone'},{days:[7]},{start:'17:00',end:'09:00'},{availability:'secret'}]){
  await assert.rejects(loadServiceContacts('https://gin.example',async()=>Response.json({contacts:[contact,{...contact,...edit,id:'different'}]})))
 }
 await assert.rejects(loadServiceContacts('https://gin.example',async()=>Response.json({contacts:[contact,contact]})))
 await assert.rejects(loadServiceContacts(undefined))
})
