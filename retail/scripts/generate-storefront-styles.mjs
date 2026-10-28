// Compile the original global stylesheet without starting/building the old app.
// The new runtime consumes plain CSS and has no Medusa/Tailwind dependency.
import {readFile,writeFile,readdir} from 'node:fs/promises'
import {createHash} from 'node:crypto'
import {createRequire} from 'node:module'
import {fileURLToPath} from 'node:url'
import path from 'node:path'

const root=fileURLToPath(new URL('../../',import.meta.url))
const original=path.join(root,'medusa-store/apps/storefront')
const require=createRequire(path.join(original,'package.json'))
const postcss=require('postcss'),tailwind=require('tailwindcss'),autoprefixer=require('autoprefixer')
const config=require(path.join(original,'tailwind.config.js'))
config.content=config.content.map(pattern=>path.resolve(original,pattern))
const relative=file=>path.relative(root,file).replaceAll('\\','/')
const normalized=bytes=>bytes.toString('utf8').replaceAll('\r\n','\n')
const hash=bytes=>createHash('sha256').update(bytes).digest('hex')
const scan=[]
async function walk(directory){
 let entries
 try{entries=await readdir(directory,{withFileTypes:true})}catch(error){if(error.code==='ENOENT')return;throw error}
 for(const entry of entries){
  const file=path.join(directory,entry.name)
  if(entry.isDirectory())await walk(file)
  else if(entry.isFile()&&/\.(?:js|ts|jsx|tsx)$/.test(entry.name))scan.push(file)
 }
}
for(const directory of ['app','pages','components','lib','modules'])await walk(path.join(original,'src',directory))
scan.sort((a,b)=>relative(a).localeCompare(relative(b),'en'))
const scanEvidence=[]
for(const file of scan)scanEvidence.push([relative(file),hash(normalized(await readFile(file)))])
const primary={}
for(const file of [path.join(original,'src/styles/globals.css'),path.join(original,'tailwind.config.js'),path.join(root,'medusa-store/pnpm-lock.yaml')])primary[relative(file)]=hash(normalized(await readFile(file)))
const output=path.join(root,'retail/storefront/src/styles/original.css')
const result=await postcss([tailwind(config),autoprefixer]).process(await readFile(path.join(original,'src/styles/globals.css'),'utf8'),{from:path.join(original,'src/styles/globals.css'),to:output,map:false})
if(result.warnings().length)throw Error(result.warnings().join('\n'))
const css='/* Compiled unchanged original globals and Tailwind config; migration source provenance in docs. */\n'+result.css
const versions={}
for(const name of ['postcss','tailwindcss','autoprefixer','@medusajs/ui-preset'])versions[name]=require(name+'/package.json').version
const manifest=JSON.stringify({format_version:1,scope:'Original global CSS and utility output only; no full-page or remote-font acceptance',primary_source_sha256:primary,content_scan:{patterns:['app','pages','components','lib','modules'].map(dir=>'src/'+dir+'/**/*.{js,ts,jsx,tsx}'),files:scanEvidence.length,sorted_path_and_normalized_sha256:hash(JSON.stringify(scanEvidence))},compiler_versions:versions,output:relative(output),output_normalized_sha256:hash(normalized(css)),runtime_dependencies_on_legacy_app:false},null,2)+'\n'
const manifestPath=path.join(root,'docs/migration/storefront-styles.json')
if(process.argv.includes('--check')){
 if(normalized(await readFile(output))!==normalized(css)||normalized(await readFile(manifestPath))!==manifest)throw Error('Original stylesheet provenance/output differs; regenerate and review')
}else{await writeFile(output,css);await writeFile(manifestPath,manifest)}
console.log(JSON.stringify({original_styles:process.argv.includes('--check')?'verified':'generated',content_files:scanEvidence.length,output_sha256:hash(normalized(css))}))
