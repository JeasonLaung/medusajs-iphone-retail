// Source: medusa-store/apps/storefront/src/modules/common/components/breadcrumbs.
// Original markup and CSS; only locale-aware link and icon imports are adapted.
import LocalizedLink from '../products/localized-link'
import {ChevronRight} from '../products/icons'
import styles from './breadcrumbs.module.css'

export default function Breadcrumbs({items,segment}:{items:readonly {label:string;href?:string}[];segment:string}){
 if(!items.length)return null
 return <nav aria-label={segment.endsWith('-zh')?'面包屑导航':'Breadcrumb'} className={styles.breadcrumbs}>
  <ol className={styles.list}>{items.map((item,index)=>{
   const current=index===items.length-1
   return <li key={`${item.href??''}-${index}`} className={styles.item}>
    {index>0&&<ChevronRight className={styles.separator} aria-hidden="true" width={14} height={14}/>}
    {current?<span className={styles.current} aria-current="page">{item.label}</span>:item.href?<LocalizedLink segment={segment} href={item.href} className={styles.link}>{item.label}</LocalizedLink>:<span>{item.label}</span>}
   </li>
  })}</ol>
 </nav>
}
