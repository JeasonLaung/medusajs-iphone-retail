// Original bodies and CSS as oracle; only framework/command imports are bridged.
import {readFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import ts from 'typescript'
const old=new URL('../../../medusa-store/apps/storefront/src/',import.meta.url)
const paths={template:'modules/cart/templates/index.tsx',summary:'modules/cart/templates/summary.tsx',line:'modules/cart/components/cart-line/index.tsx',interactions:'modules/cart/components/cart-interactions/index.tsx',copy:'modules/cart/i18n.ts',scroll:'modules/common/components/page-entry-scroll/index.tsx'}
const local=path=>fileURLToPath(new URL(path,import.meta.url)).replaceAll('\\','/')
export function legacyCart(){return {name:'legacy-cart-oracle',resolveId(id){if(id.startsWith('cart-oracle:'))return '\0'+id+'.js'},async load(id){
 if(!id.startsWith('\0cart-oracle:'))return
 const key=id.slice('\0cart-oracle:'.length,-3)
 if(key==='icons')return `export {default as Minus} from ${JSON.stringify(fileURLToPath(new URL('node_modules/@medusajs/icons/dist/esm/minus.js',new URL('../../../medusa-store/apps/storefront/',import.meta.url))).replaceAll('\\','/'))};export {default as Plus} from ${JSON.stringify(fileURLToPath(new URL('node_modules/@medusajs/icons/dist/esm/plus.js',new URL('../../../medusa-store/apps/storefront/',import.meta.url))).replaceAll('\\','/'))}`
 if(!Object.hasOwn(paths,key))throw Error('Unknown cart oracle')
 let text=await readFile(new URL(paths[key],old),'utf8')
 const aliases={'../components/cart-line':'cart-oracle:line','../components/cart-interactions':'cart-oracle:interactions','../cart-interactions':'cart-oracle:interactions','./summary':'cart-oracle:summary','../i18n':'cart-oracle:copy','../../i18n':'cart-oracle:copy','../styles.module.css':fileURLToPath(new URL('modules/cart/styles.module.css',old)).replaceAll('\\','/'),'../../styles.module.css':fileURLToPath(new URL('modules/cart/styles.module.css',old)).replaceAll('\\','/'),'@lib/data/cart':local('./cart-fixture/src/commands.ts'),'@lib/util/checkout-step':local('./cart-fixture/src/checkout.ts'),'@lib/util/product-variants':'legacy:variants','@modules/products/i18n':'legacy:copy','@modules/common/components/grade-badge':'legacy:badge','@modules/common/components/page-entry-scroll':'cart-oracle:scroll','@modules/common/components/localized-client-link':local('./presentation-fixture/src/link.tsx'),'next/image':local('./presentation-fixture/src/next-image.tsx'),'@medusajs/icons':'cart-oracle:icons'}
 for(const [a,b] of Object.entries(aliases))text=text.replaceAll('"'+a+'"',JSON.stringify(b))
 return ts.transpileModule(text,{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText
}}}
