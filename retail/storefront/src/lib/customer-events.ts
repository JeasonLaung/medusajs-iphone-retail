const event='retail:customer-changed',key='retail.customer.change.v1'

// Storage contains an opaque change marker only. Every observer re-fetches
// /me; neither a browser event nor localStorage can assert a logged-in user.
export function customerChanged(){
 if(typeof window==='undefined')return
 window.dispatchEvent(new Event(event))
 try{window.localStorage.setItem(key,crypto.randomUUID())}catch{/* Current-tab updates still work when storage is unavailable. */}
}
export function watchCustomer(callback:()=>void){
 if(typeof window==='undefined')return ()=>{}
 const storage=(e:StorageEvent)=>{if(e.key===key||e.key===null)callback()}
 const visible=()=>{if(document.visibilityState==='visible')callback()}
 window.addEventListener(event,callback)
 window.addEventListener('storage',storage)
 window.addEventListener('pageshow',callback)
 document.addEventListener('visibilitychange',visible)
 return ()=>{
  window.removeEventListener(event,callback);window.removeEventListener('storage',storage)
  window.removeEventListener('pageshow',callback);document.removeEventListener('visibilitychange',visible)
 }
}
