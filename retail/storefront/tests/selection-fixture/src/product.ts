import type {Product,Variant} from '../../../src/lib/products.ts'

const variant = (id: string,value: string,slug: string | null,amount: string,can = true): Variant => ({
 id,title:value,sku:id,thumbnail:null,variant_rank:0,options:[{id:`value-${value}`,option_id:'storage',value}],images:[{id:`image-${value}`,url:`/images/${value}.svg`,rank:0}],configuration_complete:true,
 calculated_price:{calculated_amount:amount,original_amount:null,currency_code:'eur',is_calculated_price_tax_inclusive:true,is_original_price_tax_inclusive:false,is_calculated_price_price_list:false,is_original_price_price_list:false,calculated_price:{price_list_type:null}},
 manage_inventory:true,allow_backorder:false,inventory_quantity:can?'10':'0',availability:{state:can?'available':'out_of_stock',can_fulfill:can,max_fulfillable_quantity:can?'10':'0'},display:{slug,demo_price_basis:null,apple_launch_price:null},
})
export const product: Product = {
 id:'test-phone',title:'iPhone 13',handle:'swappie-iphone-13',subtitle:null,description:null,thumbnail:null,collection_id:null,
 options:[{id:'storage',title:'Storage',values:['128','256','512','1024'].map(value=>({id:`value-${value}`,value,rank:0}))}],
 images:[{id:'model-image',url:'/images/model.svg',rank:0}],
 variants:[variant('v128','128','iphone-13-128gb-blue-S85','9007199254740993.005'),variant('v256','256','iphone-13-256gb-blue-S85','9007199254740993.015'),variant('v512','512',null,'0'),variant('v1024','1024','iphone-13-1tb-blue-S85','1000',false)],
 display:{colors:[],model_specs:null,launch_prices:[],source:null},
}
