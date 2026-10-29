import AccountAttachment,{hasCustomerHint} from './account-attachment'
import {customerClient,CustomerError} from '../../lib/customer-client'
import {useEffect,useState,Fragment} from 'react'
import {Alert,Spin,Table,Divider} from 'antd'
import {browserCartClient,type OrderConfirmation} from '../../lib/cart-client'
import {getAccountCopy} from '../account/i18n'
import {getCheckoutCopy,paymentTitle} from '../checkout/i18n'
import {Heading,Text,Button} from '../account/ui'
import Theme from '../products/storefront-theme'
import {orderUnitAmount,orderHasDiscount} from '../../lib/order-display'
import {formatCartMoney,isZeroAmount} from '../cart/types'

function Details({order,segment}:{order:OrderConfirmation;segment:string}){
 const {t,locale}=getAccountCopy(segment)
 return <div className="py-6 min-h-[calc(100vh-64px)]"><div className="content-container flex flex-col justify-center items-center gap-y-10 h-full w-full"><div className="flex flex-col gap-4 h-full bg-white w-full py-10" data-testid="order-complete-container">
  <AccountAttachment orderID={order.id} segment={segment}/>
  <Heading level="h1" className="flex flex-col gap-y-3 text-ui-fg-base text-3xl mb-4"><span>{t('orderThankYou')}</span><span>{t('orderPlaced')}</span></Heading>
  <div><Text className="mt-2">{t('orderDate')} <span data-testid="order-date">{new Date(order.created_at).toLocaleDateString(locale)}</span></Text><Text className="mt-2 text-ui-fg-interactive">{t('orderNumberLabel')}<span data-testid="order-id">{order.custom_display_id||order.display_id}</span></Text></div>
  <Heading level="h2" className="flex flex-row text-3xl-regular">{t('orderSummary')}</Heading>
  <SavedOrderSections order={order} segment={segment}/>
  <div><Heading level="h2">{t('afterSales')}</Heading><Text>{t('afterSalesDescription')}</Text><Button disabled>{t('requestAfterSales')}</Button><Text>{t('afterSalesUnavailable')}</Text></div>
 </div></div></div>
}

export function SavedOrderSections({order,segment,account=false}:{order:OrderConfirmation;segment:string;account?:boolean}){
 const {t,locale}=getAccountCopy(segment),checkout=getCheckoutCopy(segment)
 const money=(amount:string)=>formatCartMoney(amount,order.amounts.currency_code,segment)
 const totals=order.amounts.totals,lines=new Map(order.amounts.lines.map(line=>[line.item_id,line]))
 const sections=[
<div data-testid="products-table"><Table rowKey="id" pagination={false} showHeader={false} size="small" dataSource={order.items} columns={[
   {key:'image',width:80,render:(_,item)=><div className="w-16">{item.thumbnail&&<img src={item.thumbnail} alt={item.product_title??item.title} width="64" height="64" className="object-contain"/>}</div>},
   {key:'product',render:(_,item)=><div className="min-w-0 break-words"><span className="font-semibold" data-testid="product-name">{item.product_title??item.title}</span><div className="text-ui-fg-subtle" data-testid="product-variant">{Object.entries(item.option_values).map(([k,v])=><div key={k}>{k}: {v}</div>)}</div></div>},
   {key:'price',align:'right',render:(_,item)=>{const line=lines.get(item.id)!,discounted=orderHasDiscount(line.totals.total,line.totals.original_total);return <div className="flex flex-col items-end"><span className="flex gap-1"><span data-testid="product-quantity">{line.quantity}</span>x <span className="flex flex-col">{discounted&&<span className="line-through text-ui-fg-muted" data-testid="product-unit-original-price">{money(orderUnitAmount(line.totals.original_total,line.quantity,order.amounts.currency_code))}</span>}<span data-testid="product-unit-price">{money(orderUnitAmount(line.totals.total,line.quantity,order.amounts.currency_code))}</span></span></span>{discounted&&<span className="line-through text-ui-fg-muted" data-testid="product-original-price">{money(line.totals.original_total)}</span>}<span className={discounted?'text-ui-fg-interactive':''} data-testid="product-price">{money(line.totals.total)}</span></div>}},
  ]}/></div>,
<div><div className="flex flex-col gap-y-2 txt-medium text-ui-fg-subtle">
   <div className="flex items-center justify-between"><span>{t('subtotalExcludingShippingTaxes')}</span><span data-testid="cart-subtotal">{money(totals.item_subtotal)}</span></div>
   <div className="flex items-center justify-between"><span>{t('shipping')}</span><span data-testid="cart-shipping">{money(totals.shipping_subtotal)}</span></div>
   {!isZeroAmount(totals.discount_subtotal)&&<div className="flex items-center justify-between"><span>{t('discount')}</span><span>- {money(totals.discount_subtotal)}</span></div>}
   <div className="flex justify-between"><span>{t('taxes')}</span><span>{money(totals.tax_total)}</span></div>
  </div><div className="h-px w-full border-b border-gray-200 my-4"/><div className="flex items-center justify-between text-ui-fg-base mb-2 txt-medium"><span>{t('total')}</span><span className="txt-xlarge-plus" data-testid="cart-total" data-value={totals.total}>{money(totals.total)}</span></div><div className="h-px w-full border-b border-gray-200 mt-4"/></div>,
<div><Heading level="h2" className="flex flex-row text-3xl-regular my-6">{t('delivery')}</Heading><div className="grid grid-cols-1 small:grid-cols-3 gap-4 break-words">
   <div className="min-w-0 flex flex-col" data-testid="shipping-address-summary"><Text className="txt-medium-plus text-ui-fg-base mb-1">{t('shippingAddress')}</Text><Text>{order.shipping_address.first_name} {order.shipping_address.last_name}</Text><Text>{order.shipping_address.address_1} {order.shipping_address.address_2}</Text><Text>{order.shipping_address.postal_code}, {order.shipping_address.city}</Text><Text>{order.shipping_address.country_code.toUpperCase()}</Text></div>
   <div className="min-w-0 flex flex-col" data-testid="shipping-contact-summary"><Text className="txt-medium-plus text-ui-fg-base mb-1">{t('contact')}</Text><Text>{order.shipping_address.phone}</Text><Text>{order.email}</Text></div>
   <div className="min-w-0 flex flex-col" data-testid="shipping-method-summary"><Text className="txt-medium-plus text-ui-fg-base mb-1">{t('method')}</Text>{order.amounts.shipping_methods.map(method=><Text key={method.id}>{method.name} ({money(method.totals.total)})</Text>)}</div>
  </div><Divider className="mt-8"/></div>,
<div><Heading level="h2" className="flex flex-row text-3xl-regular my-6">{t('paymentHeading')}</Heading><div className="flex items-start gap-x-1 w-full"><div className="flex flex-col w-1/3"><Text className="txt-medium-plus text-ui-fg-base mb-1">{checkout.paymentMethod}</Text><Text data-testid="payment-method">{paymentTitle(order.payment.provider_id,segment)}</Text></div><div className="flex flex-col w-2/3"><Text className="txt-medium-plus text-ui-fg-base mb-1">{checkout.paymentDetails}</Text><Text data-testid="payment-amount">{t('paymentAmountAt',{amount:money(order.payment.amount),date:new Date(order.payment.created_at).toLocaleString(locale)})}</Text></div></div><Divider className="mt-8"/></div>
 ]
 return <>{(account?[sections[0],sections[2],sections[1],sections[3]]:sections).map((section,index)=><Fragment key={index}>{section}</Fragment>)}
 </>
}

export default function Confirmation({segment,orderID}:{segment:string;orderID:string}){
 const [order,setOrder]=useState<OrderConfirmation|null>(null),[status,setStatus]=useState('loading'),[retry,setRetry]=useState(0)
 const {t}=getAccountCopy(segment),copy=getCheckoutCopy(segment)
 useEffect(()=>{let active=true;setOrder(null);setStatus('loading');(async()=>{if(hasCustomerHint()){try{return await customerClient().order(orderID)}catch(e){if(!(e instanceof CustomerError)||![401,404].includes(e.status))throw e}}return browserCartClient().confirmation(orderID)})().then(order=>{if(active){setOrder(order);setStatus(order?'ready':'missing')}}).catch(()=>{if(active)setStatus('error')});return()=>{active=false}},[orderID,retry])
 return <Theme locale={segment}>{order?<Details order={order} segment={segment}/>:<div className="content-container py-16" data-testid="order-confirmation-state" data-status={status}>{status==='loading'?<Spin/>:<><Alert type="error" message={status==='missing'?copy.paymentInvalidTitle:t('error')}/><Button onClick={()=>setRetry(n=>n+1)}>{t('retry')}</Button></>}</div>}</Theme>
}
