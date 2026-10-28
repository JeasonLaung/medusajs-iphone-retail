// Original footer body is the oracle; only server data/framework imports bridge.
import {readFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {createRequire} from 'node:module'
import ts from 'typescript'
const old=new URL('../../../medusa-store/apps/storefront/src/',import.meta.url)
const target=new URL('../src/',import.meta.url)
const path=url=>fileURLToPath(url).replaceAll('\\','/')
const oldRequire=createRequire(new URL('../../../medusa-store/apps/storefront/package.json',import.meta.url))
export function legacyFooter(){return {name:'legacy-footer-oracle',resolveId(id){if(id.startsWith('footer-oracle:'))return '\0'+id+'.js'},async load(id){
 if(!id.startsWith('\0footer-oracle:'))return
 const key=id.slice('\0footer-oracle:'.length,-3)
 if(key==='ui')return `import React from 'react';export {default as clx} from ${JSON.stringify(oldRequire.resolve('clsx').replace(/clsx\.js$/, 'clsx.mjs').replaceAll('\\','/'))};export function Text({className,children}){return <p className={'txt-medium '+className}>{children}</p>}`.replace('return <p className={\'txt-medium \'+className}>{children}</p>','return React.createElement("p",{className:"txt-medium "+className},children)')
 let source
 if(key==='brand')source=(await readFile(new URL('modules/common/components/brand-logo/index.tsx',old),'utf8')).replace('import Image from "next/image"','').replace('<Image ','<img ')
 else if(key==='footer'){
  source=await readFile(new URL('modules/layout/templates/footer/index.tsx',old),'utf8')
  source=source.slice(source.indexOf('  const groups ='))
  source=`import {Text,clx} from 'footer-oracle:ui';import BrandLogo from 'footer-oracle:brand';import LocalizedClientLink from ${JSON.stringify(path(new URL('components/products/localized-link.tsx',target)))};import {getLayoutCopy} from ${JSON.stringify(path(new URL('modules/layout/i18n.ts',old)))};import {storeSeriesUrl} from ${JSON.stringify(path(new URL('lib/util/store-series-url.ts',old)))};import styles from ${JSON.stringify(path(new URL('modules/layout/templates/footer/styles.module.css',old)))};export default function Footer({data,segment,year}){const {collections,iphoneCategory}=data;const copy=getLayoutCopy(segment.endsWith('-zh')?'zh-CN':'en');`+source
  source=source.replaceAll('<LocalizedClientLink','<LocalizedClientLink segment={segment}').replace('{new Date().getFullYear()}','{year}')
 }else throw Error('Unknown footer oracle')
 return ts.transpileModule(source,{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ESNext}}).outputText
}}}
