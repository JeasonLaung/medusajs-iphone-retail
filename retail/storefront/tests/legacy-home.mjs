// Read original bodies/CSS from disk. Only async data and Next/import bridges
// are replaced by the same artificial data used by the new home component.
import {readFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import ts from 'typescript'
const source=new URL('../../../medusa-store/apps/storefront/src/',import.meta.url)
const local=p=>fileURLToPath(new URL(p,import.meta.url)).replaceAll('\\','/')
const files={gallery:'modules/home/components/series-gallery/index.tsx',carousel:'modules/home/components/home-carousel/index.tsx',rail:'modules/home/components/series-rail/index.tsx',reviews:'modules/home/components/home-reviews/index.tsx',blog:'modules/home/components/blog-section/index.tsx',rating:'modules/common/components/rating/index.tsx'}
export function legacyHome(){return {name:'legacy-home-source',resolveId(id){if(id.startsWith('legacy-home:'))return '\0'+id+'.js'},async load(id){
 if(!id.startsWith('\0legacy-home:'))return
 const key=id.slice('\0legacy-home:'.length,-3),path=files[key]
 if(!path)throw new Error('Unknown home source')
 let s=await readFile(new URL(path,source),'utf8')
 if(key==='gallery'){
  s=s.replace('import { listCategories } from "@lib/data/categories"',`import { categories } from ${JSON.stringify(local('./home-fixture/src/data.ts'))}`)
  const start=s.indexOf('  const categories = await listCategories('),end=s.indexOf('  const parent =',start)
  s=s.slice(0,start)+s.slice(end)
  s=s.replace('export default async function','export default function').replace('import { Suspense } from "react"','')
  s=s.replace(/<Suspense fallback=\{<section.*?<\/section>\}>/,'').replace('</Suspense>','')
 }
 if(key==='reviews'){
  s=s.replace('import { listProductReviews } from "@lib/data/product-reviews"',`import { data } from ${JSON.stringify(local('./home-fixture/src/data.ts'))}`)
  s=s.replace('export default async function','export default function')
  const start=s.indexOf('  const pages ='),end=s.indexOf('  return <section',start)
  s=s.slice(0,start)+'  const {reviews,unavailable}=data.reviews\n'+s.slice(end)
 }
 const aliases={'@medusajs/icons':local('../src/components/products/icons.tsx'),'next/image':local('./presentation-fixture/src/next-image.tsx'),'@modules/common/components/localized-client-link':local('../src/components/home/localized-link.tsx'),'@modules/home/i18n':local('../src/components/journal/i18n.ts'),'@lib/util/store-series-url':fileURLToPath(new URL('lib/util/store-series-url.ts',source)).replaceAll('\\','/'),'@modules/common/components/rating':'legacy-home:rating','@modules/home/components/home-carousel':'legacy-home:carousel','@modules/home/components/series-rail':'legacy-home:rail','@modules/home/components/home-reviews':'legacy-home:reviews','@modules/home/components/blog-section':'legacy-home:blog'}
 for(const [a,b]of Object.entries(aliases))s=s.replaceAll('"'+a+'"',JSON.stringify(b))
 for(const css of ['./styles.module.css','../series-gallery/styles.module.css','../home-sections.module.css'])s=s.replaceAll('"'+css+'"',JSON.stringify(fileURLToPath(new URL(css,new URL(path,source))).replaceAll('\\','/')))
 return ts.transpileModule(s,{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText
}}}
