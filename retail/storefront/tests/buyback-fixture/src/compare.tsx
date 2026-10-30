import {useEffect,useState} from 'react'
import Original from 'legacy-buyback:requests'
import Current from '../../../src/components/sell/buyback-requests'
import {AccountContext} from '../../../src/components/account/context'
import Theme from '../../../src/components/products/storefront-theme'
export default function Compare({segment,id,reference}:{segment:string;id:string;reference:boolean}){
 const [ready,setReady]=useState(false)
 useEffect(()=>setReady(true),[])
 const Component=reference?Original:Current
 return <AccountContext segment={segment}><Theme locale={segment.endsWith('-zh')?'zh-CN':'en-GB'}><div data-testid="buyback-compare" data-ready={String(ready)}><Component locale={segment.endsWith('-zh')?'zh-CN':'en'} requestId={id||undefined} signedIn/></div></Theme></AccountContext>
}
