import {useEffect,useState} from 'react'
import {Navigation} from '../../presentation-fixture/src/navigation'
import Header from '../../../src/components/layout/header'
import OriginalHeader from 'header-oracle:header'
import type {Region} from '../../../src/lib/regions'
import type {PreferenceTarget} from '../../../src/lib/preferences'
const regions:Region[]=[{id:'eu',name:'Europe',currency_code:'eur',automatic_taxes:false,countries:['dk','fr','de','it','es','se','gb'].map(code=>({iso_2:code,iso_3:code,num_code:'1',name:code,display_name:code,region_id:'eu'}))}]
export default function Probe({reference,segment,pathname,mode}:{reference:boolean;segment:string;pathname:string;mode:string}){
 const [ready,setReady]=useState(false),[saved,setSaved]=useState('')
 useEffect(()=>{setReady(true);const listener=(event:Event)=>setSaved(JSON.stringify((event as CustomEvent).detail));window.addEventListener('preferences-saved',listener);return()=>window.removeEventListener('preferences-saved',listener)},[])
 const props={regions:mode==='empty'?[]:regions,locales:[],locale:segment.endsWith('-zh')?'zh-CN':'en',cartCount:mode==='zero'?'0':mode==='large'?'900719925474099312345':'3',segment,pathname,isLoggedIn:false as const}
 async function select(target:PreferenceTarget){await fetch('/test-preferences',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(target)}).then(r=>{if(!r.ok)throw Error('Artificial preferences error')});setSaved(JSON.stringify(target))}
 return <Navigation.Provider value={{segment,location:pathname}}><div data-testid="ready" data-ready={ready}>{reference?<OriginalHeader {...props} cartCount={Number(props.cartCount)}/>:<Header {...props} onSelect={mode==='closed'?null:select}/>}</div><main style={{height:1200}}><output data-testid="saved">{saved}</output></main></Navigation.Provider>
}
