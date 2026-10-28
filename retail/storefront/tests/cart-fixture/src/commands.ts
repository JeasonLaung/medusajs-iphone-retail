// Artificial UI boundary only; actual Gin acceptance uses separate fixtures.
async function command(kind:'quantity'|'remove',lineId:string,quantity?:number){
 const response=await fetch('/test-cart',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({kind,lineId,quantity})})
 if(!response.ok)throw Error('Artificial command failure')
 window.dispatchEvent(new CustomEvent('cart-fixture-done',{detail:{kind,lineId,quantity}}))
}
export const updateLineItem=({lineId,quantity}:{lineId:string;quantity:number})=>command('quantity',lineId,quantity)
export const deleteLineItem=(lineID:string)=>command('remove',lineID)
