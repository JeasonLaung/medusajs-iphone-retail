import assert from 'node:assert/strict'
import {execFileSync} from 'node:child_process'
import {writeFileSync} from 'node:fs'
import {resolve} from 'node:path'

export async function verifyCustomerEventMail({admin,origin,logs,expect,id,customerOwner=process.env.ACCOUNT_TEST_OWNER,expectedKinds,safety=true}){
 await admin.goto(origin+'/mailboxes')
 await admin.getByRole('button',{name:'Add mailbox',exact:true}).click()
 const drawer=admin.getByRole('dialog')
 for(const [name,value] of Object.entries({'Sender name':'Windows mail fixture','Sender email':'windows-notify@example.invalid','Reply-to email':'windows-reply@example.invalid','Resend API Key':'re_windows_isolated_mail_key'}))await drawer.getByLabel(name,{exact:true}).fill(value)
 await drawer.getByRole('button',{name:'Save',exact:true}).click();await expect(drawer).not.toBeVisible()
 const dispatch=phase=>{
  const result=execFileSync(process.env.WINDOWS_MAIL_FIXTURE,['-test.run=^TestWindowsCustomerEventMail$','-test.v'],{env:{...process.env,WINDOWS_MAIL_PHASE:phase,WINDOWS_MAIL_RESOURCE:id,WINDOWS_MAIL_RECIPIENT_OWNER:customerOwner},encoding:'utf8',timeout:60000})
  writeFileSync(resolve(logs,'dispatch-'+phase+'.log'),result)
 }
 dispatch('event-reject');await admin.reload()
 let jobs=admin.getByTestId('event-mail-jobs')
 const failed=jobs.getByRole('row').filter({hasText:'Failed'})
 await expect(failed).toHaveCount(1);await expect(failed).toContainText('provider_rejected');await expect(failed).toContainText(id)
 const requests=[]
 await admin.route('**/customer-jobs/*/retry',async route=>{
  requests.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200)
  if(requests.length===1)await route.abort('failed');else await route.fulfill({response})
 })
 await failed.getByRole('button',{name:'Queue again',exact:true}).click();await admin.getByRole('button',{name:'OK',exact:true}).click()
 await expect(jobs.getByText('The retry result is not confirmed. Retry this same operation.',{exact:true})).toBeVisible()
 // Only the uncertain command remains enabled; all unrelated rows are blocked.
 await jobs.locator('button').filter({hasText:'Queue again'}).and(jobs.locator('button:not([disabled])')).click()
 await admin.unrouteAll({behavior:'wait'});assert.equal(requests.length,2);assert.deepEqual(requests[0],requests[1])
 dispatch('event-sent');dispatch('event-drain');if(safety)dispatch('event-safety');await admin.reload()
 jobs=admin.getByTestId('event-mail-jobs');await expect(jobs.getByText('Failed',{exact:true})).toHaveCount(0)
 const reply=await admin.request.get(origin+'/api/admin/notification-mail/customer-jobs?offset=0');assert.equal(reply.status(),200)
 const raw=await reply.text(),body=JSON.parse(raw);assert.ok(body.count>0);if(expectedKinds)assert.deepEqual(body.jobs.map(j=>j.kind).sort(),[...expectedKinds].sort());else assert.equal(body.count,5);assert.equal(body.jobs.every(j=>j.status==='sent'),true)
 for(const secret of ['request_envelope','sender_fingerprint','recipient_fingerprint','provider_id','windows-notify@','example.invalid'])assert.ok(!raw.includes(secret))
 assert.equal(new Set(body.jobs.map(j=>j.id)).size,body.jobs.length)
 await admin.screenshot({path:resolve(logs,'event-mail-admin-sent.png'),fullPage:true})
 return {business_event_mail_jobs:body.count,event_mail_rejected_retry_sent:true,event_mail_lost_reply_same_command:true,event_mail_frozen_message_and_key:true,event_mail_transport_isolated:true,event_mail_recipient_sender_ownership_expiry_guards:safety,real_email_delivery:false}
}

export const verifyBuybackMail=verifyCustomerEventMail
