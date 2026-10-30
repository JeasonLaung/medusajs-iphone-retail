import {useMemo} from 'react'
import {useAccountContext} from '../account/context'
export function useBuybackAPI(){
 const {api}=useAccountContext()
 return useMemo(()=>({getBuybackPhoto:api.buybackPhoto,getBuybackRequest:api.buybackRequest,listBuybackRequests:api.buybackRequests,updateBuybackRequest:api.updateBuybackRequest,uploadBuybackPhoto:api.uploadBuybackPhoto}),[api])
}
