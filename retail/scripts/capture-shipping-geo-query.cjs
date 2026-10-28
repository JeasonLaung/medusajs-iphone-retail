// Offline SQL construction from the installed original models and query path.
// connect:false; no app container, environment, database or provider is used.
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict')
const {createRequire}=require('node:module')
const root=path.resolve(__dirname,'../..'),backend=createRequire(path.join(root,'medusa-store/apps/backend/package.json'))
const resolve=createRequire(backend.resolve('@medusajs/medusa/core-flows'))
const utils=resolve('@medusajs/framework/utils'),pg=resolve('@medusajs/deps/mikro-orm/postgresql')
const fulfillment=path.dirname(resolve.resolve('@medusajs/fulfillment'))
const models=require(path.join(fulfillment,'models/index.js'))
const Service=require(path.join(fulfillment,'services/fulfillment-module-service.js')).default
async function main(){
 const orm=await pg.MikroORM.init({entities:utils.toMikroOrmEntities(Object.values(models)),driver:pg.PostgreSqlDriver,dbName:'artificial_shipping',connect:false,discovery:{disableDynamicFileAccess:true}})
 const cases=[]
 try{
  for(const [name,address] of [
   ['empty',{}],['country',{country_code:'dk'}],
   ['postal-without-ancestors',{country_code:'dk',postal_expression:'00123'}],
   ['full-address',{country_code:'dk',province_code:'01',city:'Århus',postal_expression:'00123'}],
   ['case-preserved',{country_code:'DK',postal_expression:'Ab 00123'}],
  ]){
   const normalized=Service.normalizeListShippingOptionsForContextParams({address,fulfillment_set_id:['artificial-set'],context:{is_return:'false',enabled_in_store:'true'}})
   const query=utils.buildQuery(normalized.filters,normalized.config)
   const qb=orm.em.createQueryBuilder('ShippingOption').select('id').where(query.where)
   const constraints=Service.buildGeoZoneConstraintsFromAddress(address)
   const sql=qb.getQuery(),params=qb.getParams()
   assert.ok(sql.includes('fulfillment_set_id'))
   if(address.postal_expression){
    assert.ok(sql.includes('"postal_expression" ='))
    assert.ok(params.includes(JSON.stringify(address.postal_expression)))
    assert.ok(!sql.includes('like')&&!sql.includes('~'))
   }
   cases.push({name,address,constraints,sql,params})
  }
 }finally{await orm.close()}
 const dep=createRequire(resolve.resolve('@medusajs/deps/mikro-orm/postgresql'))
 const utilRoot=path.dirname(createRequire(backend.resolve('@medusajs/framework/utils')).resolve('@medusajs/utils'))
 const knex=path.dirname(dep.resolve('@mikro-orm/knex')),core=path.dirname(dep.resolve('@mikro-orm/core')),postgres=path.dirname(dep.resolve('@mikro-orm/postgresql'))
 const sources=[path.join(fulfillment,'services/fulfillment-module-service.js'),path.join(fulfillment,'models/geo-zone.js'),path.join(utilRoot,'modules-sdk/build-query.js'),path.join(knex,'query/QueryBuilderHelper.js'),path.join(knex,'query/ScalarCriteriaNode.js'),path.join(core,'utils/QueryHelper.js'),path.join(postgres,'PostgreSqlPlatform.js')]
 const hashes=Object.fromEntries(sources.map(f=>[path.relative(root,f).replaceAll('\\','/'),crypto.createHash('sha256').update(fs.readFileSync(f,'utf8').replaceAll('\r\n','\n')).digest('hex')]))
 const text=JSON.stringify({format_version:1,scope:'Original offline SQL construction only; no PostgreSQL execution, MySQL qualification, inventory or provider acceptance',sources:hashes,cases},null,2)+'\n'
 const target=path.join(root,'docs/migration/fixtures/shipping-geo-query.json')
 assert.ok(process.argv.length<=3&&(!process.argv[2]||process.argv[2]==='--check'))
 if(process.argv[2])assert.equal(fs.readFileSync(target,'utf8').replaceAll('\r\n','\n'),text)
 else fs.writeFileSync(target,text)
 console.log(JSON.stringify({original_geo_queries:cases.length,database_connected:false}))
}
main().catch(error=>{console.error(error.message);process.exitCode=1})
