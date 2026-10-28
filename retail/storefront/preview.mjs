// Dedicated loopback visual demo; never starts the legacy services or a database.
import {dev} from 'astro'
import react from '@astrojs/react'
import {fileURLToPath} from 'node:url'
const server=await dev({root:new URL('./preview/',import.meta.url),configFile:false,output:'server',integrations:[react()],publicDir:fileURLToPath(new URL('./public/',import.meta.url)),vite:{resolve:{dedupe:['react','react-dom']}},devToolbar:{enabled:false},server:{host:'127.0.0.1',port:9300},logLevel:'info'})
console.log(`MIGRATION_PREVIEW http://127.0.0.1:${server.address.port}/dk-zh/preview`)
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,async()=>{await server.stop();process.exit(0)})
