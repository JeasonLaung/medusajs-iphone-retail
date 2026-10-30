import {useEffect,useState} from 'react'
import type {SellModel} from '../../lib/sell-models'
import {validQuestionnaireRoute} from '../../lib/sell-questionnaire'
import {AccountContext,useAccountContext} from '../account/context'
import SellFlow from './sell-flow'
import {FlowProgressProvider} from './flow-progress'
import {SellNavigation} from './navigation'
import {getSellCopy} from './i18n'
function SessionFlow({models,steps,locale}:{models:SellModel[];steps:string[];locale:string}){
 const {api}=useAccountContext()
 const [customer,setCustomer]=useState<Awaited<ReturnType<typeof api.current>>>(null)
 useEffect(()=>{let active=true;api.current().then(c=>{if(active)setCustomer(c)}).catch(()=>{});return()=>{active=false}},[api])
 if(!validQuestionnaireRoute(steps,models))return <h1>{getSellCopy(locale).unavailable}</h1>
 return <SellFlow models={models} steps={steps} locale={locale} customer={customer}/>
}
export default function Questionnaire({segment,models,steps}:{segment:string;models:SellModel[];steps:string[]}){
 const locale=segment.endsWith('-zh')?'zh-CN':'en'
 return <AccountContext segment={segment}><FlowProgressProvider><SellNavigation segment={segment} initialSteps={steps}>{steps=><SessionFlow models={models} steps={steps} locale={locale}/>}</SellNavigation></FlowProgressProvider></AccountContext>
}
