import en from "./json/en.json"
import zhCN from "./json/zh-CN.json"
import mailEn from "./json/mail-en.json"
import mailZh from "./json/mail-zh.json"

export default {
  en: { translation: { ...en, mail: mailEn } },
  zhCN: { translation: { ...zhCN, mail: mailZh } },
}
