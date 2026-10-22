import serviceEn from "./json/customer-service-en.json"
import serviceZh from "./json/customer-service-zh.json"
import en from "./json/en.json"
import zhCN from "./json/zh-CN.json"
import mailEn from "./json/mail-en.json"
import mailZh from "./json/mail-zh.json"
import reviewsEn from "./json/reviews-en.json"
import reviewsZh from "./json/reviews-zh.json"

import warehouseEn from "./json/warehouse-en.json"
import warehouseZh from "./json/warehouse-zh.json"

import webhooksEn from "./json/webhooks-en.json"
import webhooksZh from "./json/webhooks-zh.json"

export default {
  en: { translation: { ...en, mail: mailEn, reviews: reviewsEn, webhooks: webhooksEn, warehouseAccess: warehouseEn, customerService: serviceEn } },
  zhCN: { translation: { ...zhCN, mail: mailZh, reviews: reviewsZh, webhooks: webhooksZh, warehouseAccess: warehouseZh, customerService: serviceZh } },
}
