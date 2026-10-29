import {CartError,type CartClient,type CheckoutPaymentOptions} from './cart-client.ts'
import type {ContactState} from './checkout-contacts.ts'
export type PaymentReviewState=ContactState&{options:CheckoutPaymentOptions}
type API=Pick<CartClient,'retrieveContacts'|'retrieveView'|'retrieveDelivery'|'paymentOptions'>
// Read persisted, authorized amounts again after creation freezes/reprices the
// cart. The confirmation screen must not keep the pre-freeze browser total.
export async function loadPaymentReview(api:API,cartID:string,minimum:string,exact=false):Promise<PaymentReviewState>{
 const [contacts,view,options]=await Promise.all([api.retrieveContacts(cartID),api.retrieveView(cartID),api.paymentOptions(cartID)])
 if(!contacts||!view||!options||contacts.cart_id!==cartID||view.amounts.cart_id!==cartID||options.cart_id!==cartID||contacts.version!==view.amounts.version||options.version!==contacts.version||!contacts.email||!contacts.shipping_address||!contacts.billing_address||!view.items.length||!view.amounts.shipping_methods.length||BigInt(contacts.version)<BigInt(minimum)||exact&&contacts.version!==minimum)throw new CartError('conflict')
 if(!options.attempt_started){
  const delivery=await api.retrieveDelivery(cartID)
  if(!delivery||!delivery.complete||delivery.version!==contacts.version||delivery.currency_code!==view.amounts.currency_code)throw new CartError('conflict')
 }
 return {contacts,view,options}
}
