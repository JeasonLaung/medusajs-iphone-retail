import {test} from 'node:test'
import assert from 'node:assert/strict'
import {seoConfig,loadSEOConfig,configuredSEO} from '../src/lib/seo-config.ts'
import {defaultSEO,jsonLDText} from '../src/lib/seo.ts'
const page={path:'/dk-en/about',title:'Override',description:'Page description',canonical:'https://canonical.example/about',og_image:'https://images.example/seo.png',hreflang:{en:'https://shop.example/dk-en/about'},structured_data:[{'@type':'Thing',name:'</script>'}],robots:'noindex,follow,noarchive'}
const raw={settings:{'seo.site_url':'https://shop.example','seo.site_name':'Store','analytics.enabled':'true','ga4.measurement_id':'G-ABC123','seo.google_search_console.enabled':'true','gsc.verification_code':'verification'},page}
test('configured source precedence, custom languages, structured data and explicit measurement flags',()=>{
 const c=seoConfig(raw,page.path),m=configuredSEO(new URL('http://localhost:9300/dk-en/about?q=1'),{title:'Default',description:'Default description'},c)
 assert.equal(m.title,'Override');assert.equal(m.description,'Page description');assert.equal(m.canonical,page.canonical);assert.deepEqual(m.languages,[{lang:'en',href:page.hreflang.en}]);assert.equal(m.openGraph.title,'Override');assert.deepEqual(m.openGraph.images,[page.og_image]);assert.deepEqual(m.structured,page.structured_data);assert.equal(m.robots,'noindex, follow, noarchive');assert.equal(m.measurement,'G-ABC123');assert.equal(m.googleVerification,'verification')
 assert.equal(configuredSEO(new URL('https://shop.example/dk-en/account'),{title:'Account'},c).robots,'noindex, nofollow')
})
test('empty page uses configured site name/origin and retains defaults; analytics remain explicit',()=>{
 const c=seoConfig({settings:{'seo.site_url':'https://shop.example','seo.site_name':'New name'},page:null},'/dk-en/about')
 const m=configuredSEO(new URL('http://localhost/dk-en/about'),{title:'Default'},c)
 assert.equal(m.title,'Default');assert.deepEqual(m.structured,[{'@context':'https://schema.org','@type':'Organization',name:'New name',url:'https://shop.example'},{'@context':'https://schema.org','@type':'WebSite',name:'New name',url:'https://shop.example'}]);assert.equal(m.canonical,'https://shop.example/dk-en/about');assert.equal(m.measurement,undefined)
 for(const settings of [{'analytics.enabled':'false','ga4.measurement_id':'G-ABC'},{'analytics.enabled':'true','ga4.measurement_id':'invalid'}])assert.equal(configuredSEO(new URL('http://localhost/dk-en/about'),{title:'Default'},{...c,settings:{...c.settings,...settings}}).measurement,undefined)
 const home=configuredSEO(new URL('http://localhost/dk-en'),{title:'Home | EuroRefurb',openGraph:{title:'Home',description:'Intro',images:[]}},c)
 assert.equal(home.title,'Home | EuroRefurb');assert.equal(home.openGraph.title,'Home')
})
test('private settings, mismatched pages and dangerous metadata URLs cannot reach the head',()=>{
 for(const value of [{...raw,settings:{...raw.settings,'google.client_secret':'secret'}},{...raw,page:{...page,path:'/other'}},{...raw,page:{...page,canonical:'javascript:alert(1)'}},{...raw,page:{...page,hreflang:{en:'https://user:secret@example.com'}}},{...raw,page:{...page,robots:'unknown'}},{...raw,page:{...page,structured_data:'script'}}])assert.throws(()=>seoConfig(value,page.path))
})
test('Gin read uses fixed path, no credentials and caller fallback remains available on failure',async()=>{
 let called=false
 const fetcher:typeof fetch=async(input,init)=>{called=true;assert.equal(String(input),'http://localhost:9100/api/store/seo?path=%2Fdk-en%2Fabout');assert.equal(init?.credentials,'omit');assert.equal(init?.redirect,'error');return Response.json(raw)}
 assert.equal((await loadSEOConfig('http://localhost:9100','/dk-en/about///',fetcher)).page?.title,'Override');assert.equal(called,true)
 const url=new URL('http://localhost:9300/dk-en/about'),defaults={title:'Still available'}
 let metadata
 try{metadata=configuredSEO(url,defaults,await loadSEOConfig('http://localhost:9100','/dk-en/about',async()=>new Response('private failure',{status:503})))}catch{metadata=defaultSEO(url,defaults)}
 assert.equal(metadata.title,'Still available')
})

test('raw configured JSON-LD preserves large integers and safely escapes script delimiters',()=>{
 const structured='{"@type":"Thing","number":900719925474099312345,"name":"</script>&"}'
 const c=seoConfig({...raw,page:{...page,structured_data:JSON.parse(structured),structured_data_json:structured}},page.path)
 const m=configuredSEO(new URL('http://localhost/dk-en/about'),{title:'Default'},c)
 assert.equal(m.structuredJSON,structured)
 const escaped=jsonLDText(m.structuredJSON!)
 assert.ok(escaped.includes('900719925474099312345'))
 assert.equal(escaped.includes('</script>'),false)
 assert.equal(JSON.parse(escaped).name,'</script>&')
 for(const value of ['null','{}','[',42])assert.throws(()=>seoConfig({...raw,page:{...page,structured_data_json:value}},page.path))
 const empty=seoConfig({...raw,page:{...page,structured_data:[],structured_data_json:'[]'}},page.path)
 assert.equal(configuredSEO(new URL('http://localhost/dk-en/about'),{title:'Default'},empty).structuredJSON,undefined)
})
