// Source: modules/layout/templates/information-page. Original content and CSS;
// service data and locale are explicit server props instead of Next loaders.
import {ArrowRight} from '../products/icons'
import Link from '../products/localized-link'
import {getLayoutCopy} from '../layout/i18n'
import styles from './styles.module.css'

export default function Information({kind,segment,children}:{kind:'about'|'help';segment:string;children?:import('react').ReactNode}){
 const copy=getLayoutCopy(segment.endsWith('-zh')?'zh-CN':'en')
 return <div className={`${styles.page} ${kind==='help'?styles.help:''}`}><div className={styles.container}>
  <p className={styles.eyebrow}>{copy[kind]}</p><h1>{copy[`${kind}Title`]}</h1><p className={styles.description}>{copy[`${kind}Description`]}</p>
  {kind==='about'&&<section id="our-focus" className={styles.section}><h2>{copy.focus}</h2><p>{copy.focusDescription}</p></section>}
  {children}
  {kind==='help'&&<div className={styles.sections}>
   <section id="shopping" className={styles.section}><h2>{copy.shopping}</h2><p>{copy.shoppingDescription}</p><Link segment={segment} href="/store">{copy.store} <ArrowRight aria-hidden="true" width={20} height={20} className="inline-block align-middle shrink-0"/></Link></section>
   <section id="account" className={styles.section}><h2>{copy.orders}</h2><p>{copy.ordersDescription}</p><Link segment={segment} href="/account">{copy.account} <ArrowRight aria-hidden="true" width={20} height={20} className="inline-block align-middle shrink-0"/></Link></section>
  </div>}
  <Link segment={segment} href="/store" className={styles.back}>{copy.back} <ArrowRight aria-hidden="true" width={20} height={20} className="inline-block align-middle shrink-0"/></Link>
 </div></div>
}
