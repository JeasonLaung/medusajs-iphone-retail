import {useMemo} from 'react'
import {InstantSearchSSRProvider,type InstantSearchServerState} from 'react-instantsearch'
import type {SearchClient} from 'instantsearch.js'
import Template,{type StoreTemplateProps} from './template'
import {createSearchClient} from './client'
import {SearchRoute} from './context'
import StorefrontTheme from '../products/storefront-theme'
export type StoreSearchProps=Omit<StoreTemplateProps,'searchClient'>&{segment:string;regionID:string;serverState?:InstantSearchServerState;serverClient?:SearchClient}
export default function StoreSearch({segment,regionID,serverState,serverClient,...props}:StoreSearchProps){
 const client=useMemo(()=>serverClient??createSearchClient(regionID,props.currencyCode),[regionID,props.currencyCode,serverClient])
 return <StorefrontTheme locale={segment}><SearchRoute segment={segment}><InstantSearchSSRProvider {...serverState}><Template {...props} searchClient={client}/></InstantSearchSSRProvider></SearchRoute></StorefrontTheme>
}
