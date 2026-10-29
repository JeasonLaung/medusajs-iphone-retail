// Order prices in the source are tax-inclusive line totals divided by quantity,
// not the raw catalog unit_price. Round the rational value to currency digits
// without converting the exact monetary strings to binary floating point.
function decimal(value:string){
 if(!/^(0|[1-9][0-9]{0,191})(\.[0-9]{1,128})?$/.test(value)||/[\r\n]/.test(value))throw Error('Invalid order decimal')
 const [whole,fraction='']=value.split('.');return {n:BigInt(whole+fraction),scale:10n**BigInt(fraction.length)}
}
export function orderUnitAmount(total:string,quantity:string,currency:string):string{
 const amount=decimal(total),count=decimal(quantity)
 if(count.n===0n||!/^[a-z]{3}$/.test(currency))throw Error('Invalid order unit amount')
 const digits=new Intl.NumberFormat('en',{style:'currency',currency}).resolvedOptions().maximumFractionDigits??2
 const scale=10n**BigInt(digits),numerator=amount.n*count.scale*scale,denominator=amount.scale*count.n
 const rounded=(numerator*2n+denominator)/(2n*denominator)
 return digits?`${rounded/scale}.${String(rounded%scale).padStart(digits,'0')}`:String(rounded)
}
export function orderHasDiscount(total:string,original:string):boolean{
 const a=decimal(total),b=decimal(original);return a.n*b.scale<b.n*a.scale
}
