import Theme from '../products/storefront-theme'
import {AccountContext} from '../account/context'
import Showcase from './showcase'

export default function UiShowcase({segment}:{segment:string}){
 const locale=segment.endsWith('-zh')?'zh-CN':'en'
 return <Theme locale={locale}><AccountContext segment={segment}><Showcase segment={segment} locale={locale}/></AccountContext></Theme>
}
