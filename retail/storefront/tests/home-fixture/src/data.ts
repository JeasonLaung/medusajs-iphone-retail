import {homeSeries} from '../../../src/lib/home'
import type {FooterData} from '../../../src/lib/footer-data'
export const categories=[{id:'iphone',name:'iPhone',handle:'iphone',parent_category_id:null},...['13','Air','17','12','16','15','14','11','SE'].map(f=>({id:'series-'+f,name:'iPhone '+f+' series',handle:'iphone-'+f.toLowerCase()+'-series',parent_category_id:'iphone',products:[]}))]
export const data={series:homeSeries({collections:[],iphoneCategory:{...categories[0],category_children:categories.slice(1)}} as FooterData),reviews:{reviews:[{id:'r',rating:4.5,title:'As described',content:'Good condition and delivery.',author:'Customer',created_at:'2026-10-29T00:00:00Z',verified_purchase:true,product:{id:'p',title:'iPhone 13',handle:'iphone-13'}}],unavailable:false}}
