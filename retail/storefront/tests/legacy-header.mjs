// Original header/region body oracle: only framework and mutation imports bridge.
import {readFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import ts from 'typescript'
const old=new URL('../../../medusa-store/apps/storefront/src/',import.meta.url)
const local=file=>fileURLToPath(new URL(file,import.meta.url)).replaceAll('\\','/')
const source={header:'modules/layout/components/store-header/index.tsx',region:'modules/layout/components/region-language/index.tsx',brand:'modules/common/components/brand-logo/index.tsx'}
export function legacyHeader(){return {name:'legacy-header-oracle',resolveId(id){if(id.startsWith('header-oracle:'))return '\0'+id+'.js'},async load(id){
 if(!id.startsWith('\0header-oracle:'))return
 const key=id.slice('\0header-oracle:'.length,-3)
 if(key==='navigation')return `export {useParams,usePathname} from ${JSON.stringify(local('./presentation-fixture/src/navigation.tsx'))};export const unstable_rethrow=()=>{}`
 if(key==='icons')return ['ChevronDown','XMark'].map((name,i)=>`export {default as ${name}} from ${JSON.stringify(local('../../../medusa-store/apps/storefront/node_modules/@medusajs/icons/dist/esm/'+['chevron-down','x-mark'][i]+'.js'))}`).join(';')
 if(key==='commands')return `export async function updateLocale(){};export async function updateRegion(country,currentPath,language){const response=await fetch('/test-preferences',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({country,currentPath,locale:language})});if(!response.ok)throw Error('Artificial preferences error');window.dispatchEvent(new CustomEvent('preferences-saved',{detail:{country,currentPath,locale:language}}))}`
 if(key==='notification')return 'export default function Notification(){return null}'
 if(!Object.hasOwn(source,key))throw Error('Unknown header oracle')
 let text=await readFile(new URL(source[key],old),'utf8')
 if(key==='header')text=text.replace('<PopoverPanel className={styles.panel}>','<PopoverPanel data-testid="nav-desktop-panel" className={styles.panel}>')
 if(key==='header')text=text.replace('<PopoverPanel className={styles.mobilePanel}', '<PopoverPanel data-testid="nav-mobile-panel" className={styles.mobilePanel}')
 const aliases={'@modules/common/components/brand-logo':'header-oracle:brand','@modules/notifications/notification-bell':'header-oracle:notification','@medusajs/icons':'header-oracle:icons','next/navigation':'header-oracle:navigation','../region-language':'header-oracle:region','@modules/common/components/localized-client-link':local('./presentation-fixture/src/link.tsx'),'@modules/layout/i18n':local('../../../medusa-store/apps/storefront/src/modules/layout/i18n.ts'),'@lib/util/storefront-route':local('../../../medusa-store/apps/storefront/src/lib/util/storefront-route.ts'),'@lib/data/locale-actions':'header-oracle:commands','@lib/data/cart':'header-oracle:commands','./styles.module.css':local('../../../medusa-store/apps/storefront/src/modules/layout/components/'+(key==='header'?'store-header':'region-language')+'/styles.module.css'),'next/image':local('./presentation-fixture/src/next-image.tsx')}
 for(const [a,b] of Object.entries(aliases))text=text.replaceAll('"'+a+'"',JSON.stringify(b))
 return ts.transpileModule(text,{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText
}}}
