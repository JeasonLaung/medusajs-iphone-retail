// Test-only source oracle. Original component bodies/CSS are read from disk;
// only framework/type imports are bridged for the isolated React runtime.
import {readFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import ts from 'typescript'
const old=new URL('../../../medusa-store/apps/storefront/src/',import.meta.url)
const local=name=>fileURLToPath(new URL('./presentation-fixture/src/'+name,import.meta.url)).replaceAll('\\','/')
const source={gallery:'modules/products/components/image-gallery/index.tsx',details:'modules/products/components/product-detail/index.tsx',variants:'lib/util/product-variants.ts',copy:'modules/products/i18n.ts'}
Object.assign(source,{actions:'modules/products/components/product-actions/index.tsx',guide:'modules/products/components/grade-guide/index.tsx',sticky:'modules/products/components/sticky-purchase-bar/index.tsx',theme:'modules/common/components/storefront-theme/index.tsx',selection:'modules/common/components/selection-row/index.tsx',badge:'modules/common/components/grade-badge/index.tsx',intersection:'lib/hooks/use-in-view.tsx',prices:'lib/util/get-product-price.ts',launch:'lib/util/get-apple-launch-price.ts',percentage:'lib/util/get-percentage-diff.ts',money:'lib/util/money.ts',inventory:'lib/util/cart-inventory.ts',colors:'modules/products/components/color-swatches/index.tsx'})
const iconRoot=new URL('../../../medusa-store/apps/storefront/node_modules/@medusajs/icons/dist/esm/',import.meta.url)
export function legacyPresentation() {
 return {name:'legacy-presentation-oracle',resolveId(id){if(id.startsWith('legacy:'))return '\0'+id+'.js'},async load(id){
  if(!id.startsWith('\0legacy:'))return
  const key=id.slice('\0legacy:'.length,-3)
  if(key==='icons')return `export {default as ChevronLeft} from 'legacy:icon-chevron-left';export {default as ChevronRight} from 'legacy:icon-chevron-right';export {default as ArrowUpRightMini} from 'legacy:icon-arrow-up-right-mini'`
  if(['icon-chevron-left','icon-chevron-right','icon-arrow-up-right-mini'].includes(key))return readFile(new URL(key.slice(5)+'.js',iconRoot),'utf8')
  if(key==='isEmpty')return ts.transpileModule(await readFile(new URL('lib/util/isEmpty.ts',old),'utf8'),{compilerOptions:{module:ts.ModuleKind.ESNext}}).outputText
  if(!Object.hasOwn(source,key))throw new Error('Unknown legacy test module')
  let text=await readFile(new URL(source[key],old),'utf8')
  const aliases={'@lib/data/cart':local('cart.ts'),'@lib/util/get-product-price':'legacy:prices','@lib/util/get-apple-launch-price':'legacy:launch','@lib/hooks/use-in-view':'legacy:intersection','@lib/util/iphone-thumbnails.json':fileURLToPath(new URL('lib/util/iphone-thumbnails.json',old)).replaceAll('\\','/'),'@modules/common/components/localized-client-link':local('link.tsx'),'@modules/common/components/grade-badge':'legacy:badge','@modules/common/components/selection-row':'legacy:selection','@modules/common/components/storefront-theme':'legacy:theme','@modules/products/i18n':'legacy:copy','../color-swatches':'legacy:colors','../grade-guide':'legacy:guide','../sticky-purchase-bar':'legacy:sticky','@lib/util/cart-inventory':'legacy:inventory','./get-percentage-diff':'legacy:percentage','./money':'legacy:money','antd/locale/en_GB':'antd/locale/en_GB.js','antd/locale/zh_CN':'antd/locale/zh_CN.js'}
  for(const [from,to] of Object.entries(aliases))text=text.replaceAll('"'+from+'"',JSON.stringify(to))
  text=text.replaceAll('"./isEmpty"','"legacy:isEmpty"')
  text=text.replaceAll('"./styles.module.css"',JSON.stringify(fileURLToPath(new URL('./styles.module.css',new URL(source[key],old))).replaceAll('\\','/')))
  for(const [from,to] of Object.entries({'@medusajs/icons':'legacy:icons','next/image':local('next-image.tsx'),'next/navigation':local('navigation.tsx'),'@lib/util/product-variants':'legacy:variants','../../i18n':'legacy:copy','../product-detail/styles.module.css':fileURLToPath(new URL('modules/products/components/product-detail/styles.module.css',old)).replaceAll('\\','/'),'./styles.module.css':fileURLToPath(new URL('modules/products/components/product-detail/styles.module.css',old)).replaceAll('\\','/')}))text=text.replaceAll('"'+from+'"',JSON.stringify(to))
  return ts.transpileModule(text,{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText
 }}
}
