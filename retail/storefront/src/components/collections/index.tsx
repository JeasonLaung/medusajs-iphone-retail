import type {CollectionPage} from '../../lib/collection-page'
import {collectionNavigation} from '../../lib/collection-page'
import type {Region} from '../../lib/regions'
import Theme from '../products/storefront-theme'
import ProductPreview from './product-preview'
import SortProducts from './sort-products'
import {Pagination} from './pagination'
import styles from './styles.module.css'

export default function Collection({data,region,segment,requestUrl}:{data:CollectionPage;region:Region;segment:string;requestUrl:string}){
 return <Theme locale={segment.endsWith('-zh')?'zh-CN':'en-GB'}><div className="flex flex-col small:flex-row small:items-start py-6 content-container" data-testid="collection-page">
  <div className={`flex flex-col gap-12 py-4 mb-8 small:px-0 pl-6 small:min-w-[250px] small:ml-[1.675rem] ${styles.sort}`}><SortProducts sortBy={data.query.sort} setQueryParams={(_key,value)=>window.location.assign(collectionNavigation(requestUrl,{sort:value as CollectionPage['query']['sort']}))} data-testid="collection-sort"/></div>
  <div className="w-full"><div className="mb-8 text-2xl-semi"><h1>{data.collection.title}</h1></div>
   <ul className="grid grid-cols-1 w-full small:grid-cols-2 gap-x-6 gap-y-8" data-testid="products-list">{data.products.map(product=><li key={product.id}><ProductPreview product={product} region={region} segment={segment}/></li>)}</ul>
   {data.totalPages>1?<Pagination page={data.query.page} totalPages={data.totalPages} requestUrl={requestUrl} data-testid="product-pagination"/>:null}
  </div>
 </div></Theme>
}
