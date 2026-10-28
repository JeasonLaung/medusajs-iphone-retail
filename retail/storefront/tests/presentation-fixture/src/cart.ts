// Artificial test boundary only; browser interception supplies every response.
export async function fixtureAdd(input:unknown){const response=await fetch('/test-cart',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(input)});if(!response.ok)throw new Error('Fixture failure');return response.json()}
export async function addToCart(input:unknown){const result=await fixtureAdd(input);return result.ok?{...result,quantities:Object.fromEntries(Object.entries(result.quantities).map(([id,n])=>[id,Number(n)]))}:result}
