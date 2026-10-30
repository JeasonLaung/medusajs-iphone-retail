// Render the actual original source; only framework/API and Ant version adapters.
import {readFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import ts from 'typescript'
const old=new URL('../../../medusa-store/apps/storefront/src/modules/sell/',import.meta.url)
const local=p=>fileURLToPath(new URL(p,import.meta.url)).replaceAll('\\','/')
export function legacyBuyback(){return {name:'legacy-buyback-source',resolveId(id){if(id==='legacy-buyback:requests')return '\0legacy-buyback:requests.js'},async load(id){
 if(id!=='\0legacy-buyback:requests.js')return
 let s=await readFile(new URL('components/buyback-requests.tsx',old),'utf8')
 s=s.replace('import { useParams } from "next/navigation"','import { useAccountContext } from '+JSON.stringify(local('../src/components/account/context.tsx')))
 s=s.replaceAll('const { countryCode } = useParams<{ countryCode: string }>()','const { segment: countryCode } = useAccountContext()')
 s=s.replace(/import \{ getBuybackPhoto[^\n]+from "@lib\/data\/buyback"/, 'import { useBuybackAPI } from '+JSON.stringify(local('../src/components/sell/request-api.ts')))
 s=s.replace('  const [source, setSource]', '  const {getBuybackPhoto}=useBuybackAPI()\n  const [source, setSource]')
 s=s.replace('  const [request, setRequest]', '  const {getBuybackRequest,updateBuybackRequest,uploadBuybackPhoto}=useBuybackAPI()\n  const [request, setRequest]')
 s=s.replace('  const [data, setData]', '  const {getBuybackRequest,listBuybackRequests}=useBuybackAPI()\n  const [data, setData]')
 const aliases={'@medusajs/icons':local('../src/components/sell/request-icons/index.ts'),'./sell-theme':local('../src/components/products/storefront-theme/index.tsx'),'../buyback-copy':fileURLToPath(new URL('buyback-copy.ts',old)).replaceAll('\\','/'),'../request.module.css':fileURLToPath(new URL('request.module.css',old)).replaceAll('\\','/')}
 for(const[a,b]of Object.entries(aliases))s=s.replaceAll('"'+a+'"',JSON.stringify(b))
 s=s.replaceAll('showIcon title=', 'showIcon message=').replace('orientation="vertical"','direction="vertical"').replace('        content:', '        description:').replace('mask={{ closable: !busy }}','maskClosable={!busy}')
 return ts.transpileModule(s,{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText
}}}
