import {defineConfig,envField} from 'astro/config'
import node from '@astrojs/node'
import react from '@astrojs/react'

export default defineConfig({
 output:'server',
 adapter:node({mode:'standalone'}),
 integrations:[react()],
 devToolbar:{enabled:false},
 // Local development hosts only. Never trust arbitrary Host/forwarded headers
 // when constructing canonical and social-image URLs in server-rendered pages.
 security:{allowedDomains:[{hostname:'127.0.0.1'},{hostname:'localhost'}]},
 env:{schema:{GIN_API_ORIGIN:envField.string({context:'server',access:'secret',optional:true}),STOREFRONT_ORIGIN:envField.string({context:'server',access:'secret',optional:true})}},
})
