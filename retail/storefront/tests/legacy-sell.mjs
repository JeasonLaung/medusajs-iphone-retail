// Original component and CSS; framework and Ant v6->v5 prop bridges only.
import {readFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import ts from 'typescript'
const old=new URL('../../../medusa-store/apps/storefront/src/modules/sell/',import.meta.url)
const local=p=>fileURLToPath(new URL(p,import.meta.url)).replaceAll('\\','/')
export function legacySell(){return {name:'legacy-sell-source',resolveId(id){if(id==='legacy-sell:landing')return '\0legacy-sell:landing.js'},async load(id){
 if(id!=='\0legacy-sell:landing.js')return
 let s=await readFile(new URL('components/sell-landing.tsx',old),'utf8')
 s=s.replace('import { useParams } from "next/navigation"','').replace('const { countryCode } = useParams<{ countryCode: string }>()','const countryCode="dk-"+(locale?.startsWith("zh")?"zh":"en")')
 const aliases={'next/image':local('./presentation-fixture/src/next-image.tsx'),'@medusajs/icons':local('../src/components/products/icons.tsx'),'./sell-theme':local('../src/components/products/storefront-theme/index.tsx'),'../i18n':fileURLToPath(new URL('i18n.ts',old)).replaceAll('\\','/'),'../buyback-copy':fileURLToPath(new URL('buyback-copy.ts',old)).replaceAll('\\','/'),'../styles.module.css':fileURLToPath(new URL('styles.module.css',old)).replaceAll('\\','/')}
 for(const [a,b]of Object.entries(aliases))s=s.replaceAll('"'+a+'"',JSON.stringify(b))
 s=s.replace('iconPlacement="end"','iconPosition="end"').replace('content: step.description','description: step.description')
 return ts.transpileModule(s,{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText
}}}
