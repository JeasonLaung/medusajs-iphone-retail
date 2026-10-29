import {Menu,Typography} from 'antd'
import type {Customer} from '../../lib/customer-client'
import {useAccountContext,useAccountCopy} from './context'
import Link from './localized-link'
import {Button} from './ui'
import User from './icons/user'
import MapPin from './icons/map-pin'
import Package from './icons/package'
import ChevronDown from './icons/chevron-down'
import Logout from './icons/arrow-right-on-rectangle'

export default function AccountNav({customer,selected,loggingOut,onLogout}:{customer:Customer;selected:string;loggingOut:boolean;onLogout:()=>void}){
 const {segment}=useAccountContext(),{t}=useAccountCopy()
 const links=[
  {key:'/account',label:t('overview'),testId:'overview-link',icon:<User/>},
  {key:'/account/profile',label:t('profile'),testId:'profile-link',icon:<User/>},
  {key:'/account/addresses',label:t('addresses'),testId:'addresses-link',icon:<MapPin/>},
  {key:'/account/orders',label:t('orders'),testId:'orders-link',icon:<Package/>},
  {key:'/account/buyback',label:t('myRequests'),testId:'buyback-link',icon:<Package/>},
 ]
 const menu=<Menu mode="inline" style={{borderInlineEnd:0,background:'transparent'}} selectedKeys={[selected]} items={[
  ...links.map(link=>({key:link.key,icon:link.icon,label:<Link href={link.key} data-testid={link.testId}>{link.label}</Link>})),
  {key:'logout',icon:<Logout/>,disabled:loggingOut,label:<span data-testid="logout-button">{t('logout')}</span>,onClick:onLogout},
 ]}/>
 return <nav aria-label={t('account')} className="mb-6 small:mb-0 small:pr-6">
  <div className="hidden small:block" data-testid="account-nav"><Typography.Title level={4}>{t('account')}</Typography.Title>{menu}</div>
  <div className="small:hidden" data-testid="mobile-account-nav">{selected==='/account'?<><Typography.Title level={4}>{t('hello',{name:customer.first_name})}</Typography.Title>{menu}</>:<Button href={`/${segment}/account`} variant="transparent" icon={<ChevronDown className="rotate-90"/>} data-testid="account-main-link">{t('account')}</Button>}</div>
 </nav>
}
