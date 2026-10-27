import {defineConfig,envField} from 'astro/config'
import node from '@astrojs/node'
import react from '@astrojs/react'

export default defineConfig({
 output:'server',
 adapter:node({mode:'standalone'}),
 integrations:[react()],
 devToolbar:{enabled:false},
 env:{schema:{GIN_API_ORIGIN:envField.string({context:'server',access:'secret',optional:true})}},
})
