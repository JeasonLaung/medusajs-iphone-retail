// Test-only source oracle. Original component bodies/CSS are read from disk;
// only framework/type imports are bridged for the isolated React runtime.
import {readFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import ts from 'typescript'
const old=new URL('../../../medusa-store/apps/storefront/src/',import.meta.url)
const local=name=>fileURLToPath(new URL('./presentation-fixture/src/'+name,import.meta.url)).replaceAll('\\','/')
const source={gallery:'modules/products/components/image-gallery/index.tsx',details:'modules/products/components/product-detail/index.tsx',variants:'lib/util/product-variants.ts',copy:'modules/products/i18n.ts'}
const iconRoot=new URL('../../../medusa-store/apps/storefront/node_modules/@medusajs/icons/dist/esm/',import.meta.url)
export function legacyPresentation() {
 return {name:'legacy-presentation-oracle',resolveId(id){if(id.startsWith('legacy:'))return '\0'+id+'.js'},async load(id){
  if(!id.startsWith('\0legacy:'))return
  const key=id.slice('\0legacy:'.length,-3)
  if(key==='icons')return `export {default as ChevronLeft} from 'legacy:icon-chevron-left';export {default as ChevronRight} from 'legacy:icon-chevron-right';export {default as ArrowUpRightMini} from 'legacy:icon-arrow-up-right-mini'`
  if(['icon-chevron-left','icon-chevron-right','icon-arrow-up-right-mini'].includes(key))return readFile(new URL(key.slice(5)+'.js',iconRoot),'utf8')
  if(!Object.hasOwn(source,key))throw new Error('Unknown legacy test module')
  let text=await readFile(new URL(source[key],old),'utf8')
  for(const [from,to] of Object.entries({'@medusajs/icons':'legacy:icons','next/image':local('next-image.tsx'),'next/navigation':local('navigation.tsx'),'@lib/util/product-variants':'legacy:variants','../../i18n':'legacy:copy','../product-detail/styles.module.css':fileURLToPath(new URL('modules/products/components/product-detail/styles.module.css',old)).replaceAll('\\','/'),'./styles.module.css':fileURLToPath(new URL('modules/products/components/product-detail/styles.module.css',old)).replaceAll('\\','/')}))text=text.replaceAll('"'+from+'"',JSON.stringify(to))
  return ts.transpileModule(text,{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText
 }}
}
