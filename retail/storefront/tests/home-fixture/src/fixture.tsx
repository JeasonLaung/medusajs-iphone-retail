import {useEffect,useState} from 'react'
import Home from '../../../src/components/home'
import Reference from 'legacy-home:gallery'
import Theme from '../../../src/components/products/storefront-theme'
import {HomeRoute} from '../../../src/components/home/localized-link'
import {getHomeCopy} from '../../../src/components/journal/i18n'
import {data} from './data'
export default function Fixture({locale,reference}:{locale:string;reference:boolean}){
 const [ready,setReady]=useState(false)
 useEffect(()=>setReady(true),[])
 const segment='dk-'+locale,copy=getHomeCopy(locale==='zh'?'zh-CN':'en')
 return <div data-testid="home-fixture" data-ready={ready}>{reference?<Theme locale={locale}><HomeRoute segment={segment}><Reference copy={copy}/></HomeRoute></Theme>:<Home segment={segment} copy={copy} data={data}/>}</div>
}
