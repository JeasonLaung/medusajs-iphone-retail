import type {ContactAddress} from '../../lib/cart-client'
// Original compare-addresses intentionally does not compare address_2.
export default function compareAddresses(a:ContactAddress,b:ContactAddress){return (['first_name','last_name','address_1','company','postal_code','city','country_code','province','phone'] as const).every(k=>a[k]===b[k])}
