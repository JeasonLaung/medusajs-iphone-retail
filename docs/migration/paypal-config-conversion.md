# PayPal 商户秘密与配置转换

2026-10-27。T006/T018 的迁移组件，不代表 PayPal 新运行时或全量领域验收。没有连接 PayPal、刷新 token、交换 onboarding code 或启动 webhook/重试任务。

## 源秘密格式修正

已核对本机安装的 `@easypayment/medusa-payment-paypal@1.3.2` 的 `src/modules/paypal/utils/secret-crypto.ts` 及实际 exports 所用 `.medusa/server/src/modules/paypal/utils/secret-crypto.js`。该插件与项目其他四域的点分格式不同：

| 来源 | 解密方式 |
| --- | --- |
| 无 enc: 前缀的旧字符串 | 明确的旧明文路径，原样保留 |
| enc:v1:iv:tag:ciphertext | SHA256(经 ECMAScript trim 的 PAYPAL_ENCRYPTION_KEY)，AES-256-GCM |
| enc:v2:salt:iv:tag:ciphertext | scrypt N=16384/r=8/p=1/32字节，16字节随机 salt，再 AES-256-GCM |

`legacysecret.NewPayPal(explicitKey).Open` 不读取环境或回退 COOKIE_SECRET。编码必须规范标准 base64，IV12/tag16、UTF-8、长度严格校验；未知 enc: 版本、缺密钥、错误密钥、篡改或格式失败均拒绝，不退回明文。不接受新 ers1. 信封冒充源明文。源 key 的 trim 明确匹配 ECMAScript（含 BOM、不删 NEL），不套用 Go 不同的空白集合。加密正文尺寸在 scrypt 前检查；参数固定，不从源值读取成本参数。

`NewConfigSecretsWithPayPal(target, fourDomainKeys, paypalReader)` 对 `seller_client_secret`、`app_access_token` 使用该专用读取器，再以 merchant-config / paypal_connections / 精确字段 / 源ID 的新信封密封。`auth_code` 的源代码直接保存 onboarding code，继续按商户字符串密封，不误用 token 加密格式。普通 `NewConfigSecrets` 保持旧明文可用，但遇到带前缀的上述两个加密字段会拒绝；修正此前将原 PayPal 密文再次当普通字符串包裹的草稿行为。

互操作证据见 [paypal-secret-interop-report.json](paypal-secret-interop-report.json)：9个虚构明文/v1/v2样本经原 TypeScript 和已安装 JS 双向解密校验，再交给 Go 解密并完整行密封18个目标字段，目标解密后等于原明文；缺旧 key 拒绝返回半行。测试文件独占创建在 Git 忽略目录，结束删除，不含真实凭据。

## 嵌套 JSON 的明确范围

`ReviewPayPalConfigJSON` 和 ConfigSecrets 目前支持固定快照的如下内容，完整保留原 JSON 字节，保存为 LONGBLOB 新信封：

- connection.metadata：空 credentials 对象和 live/sandbox active_environment；active_environment 如存在必须等于来源 environment。
- settings.data.api_details：大写三字符 currency_code，可选结构合法的 storefront_url。
- settings.data.onboarding_config：bn_code、backend_url、seller_nonce、partner_js_url、partner_service_url、partner_merchant_id_live/sandbox，均按原文商户设置保存。URL/nonce 不被执行或用于换取凭据。

SQL NULL 与 JSON `null` 分开；未知字段、重复键、非法 Unicode、类型不符一律拒绝。**非空 credentials.<environment> 仍不支持**：其中 client_secret 可能是旧 v1/v2，需另行逐字段解密投影，不能整体封装旧密文假装完成。其他后台可配置 section、metrics/webhook JSON 也未因此开放。

真实 v2 离线验证见 [paypal-config-v2-report.json](paypal-config-v2-report.json)：完整159表流校验后，1条connection和1条settings转换成功，两个JSON临时信封往返及错行AAD拒绝通过，3个秘密字段明确SQL NULL。快照没有实际非空 PayPal 凭据，人工互操作不能当作真实凭据验证；未将真实新密文或 key 持久化。

```powershell
node retail/scripts/test-paypal-secret-interop.cjs
python retail/scripts/audit-paypal-config.py --snapshot logs/gin-migration/source-v2-20261027-payment-rows.ndjson
```

仍缺非空嵌套凭据/其他设置 section、merchant配置的持久化副作用hold、真实SQL演练、密钥备份/恢复、读取权限及Gin provider运行时。来源插件自动 ensureSettingsDefaults、ensureWebhookRegistration 等动作绝不在转换时调用。全部历史付款与原账号限制继续有效，T006/T018/T035不验收。
