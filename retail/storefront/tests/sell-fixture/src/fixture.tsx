import {useEffect,useState} from 'react'
import Landing from '../../../src/components/sell'
import Reference from 'legacy-sell:landing'
export default function Fixture({locale,reference}:{locale:string;reference:boolean}){
 const [ready,setReady]=useState(false)
 useEffect(()=>setReady(true),[])
 return <div data-testid="sell-fixture" data-ready={ready}>{reference?<Reference locale={locale}/>:<Landing locale={locale} segment={'dk-'+locale}/>}</div>
}
