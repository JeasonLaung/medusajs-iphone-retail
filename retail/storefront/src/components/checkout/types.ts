import type {ContactAddress} from '../../lib/cart-client'
import type {Region} from '../../lib/regions'
import type {Address} from '../../lib/customer-client'
export type CheckoutCart={email:string;shipping_address:ContactAddress|null;billing_address:ContactAddress|null;region:Region}
export type CheckoutCustomer={email:string;addresses:Address[]}
export type CheckoutStep='address'|'delivery'|'payment'|'review'
