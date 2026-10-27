# 商户字符串秘密的Go转换

日期：2026-10-27。T006/T018的转换组件；不是完整导入器、正式数据库迁移或provider接入。

## 源解密

`internal/legacysecret`仅用于离线读取旧商户秘密。显式提供mail/seo/webhook/mollie域的源配置字符串，按现有四个TypeScript实现使用SHA256得到AES-256-GCM密钥；读取base64(iv).base64(tag).base64(ciphertext)，无源AAD。不会读.env、自动回退COOKIE_SECRET、试探新旧格式、猜测明文或调用外部服务。离线调用方须显式选择旧环境中的域密钥或旧COOKIE_SECRET，新业务Ring必须使用独立生成的32字节密钥。

每个非空值必须为三个规范标准base64段，IV12字节、tag16字节，正文不超过1MiB；编码总长度上限2MiB。拒绝非规范base64、错段数/尺寸、错误密钥、tag篡改、非UTF-8结果、缺域密钥及默认supersecret。Node原解码器更宽松，不能将拒绝的历史值自动规范化后继续导入；此类值须私下审查。旧空字符串仍表示空值，无需旧密钥即可处理；缺失的非空配置从不回退。旧密文没有行/列/域AAD，域配置名不同不证明旧密文不能跨域互换。

## 12字段显式入口

`rowimport.NewConfigSecrets(targetRing, legacyKeys).Convert(table, row)`复用完整Convert的字段覆盖、类型/长度、NULL、raw配对和CHECK规则，只为以下protected_string字段提供转换：

| 源字段 | 处理 | 新AAD purpose |
| --- | --- | --- |
| mail_account.api_key/webhook_secret | mail域认证解密 | merchant-config |
| seo_provider.api_key | seo域认证解密 | merchant-config |
| seo_setting.value | is_secret=true使用seo解密；false保留原文字节 | merchant-config |
| webhook_endpoint.url/secret/token | webhook域认证解密，URL不试发 | merchant-config |
| mollie_config.api_key | mollie域认证解密，不验证商户凭据 | merchant-config |
| paypal_connection.auth_code/seller_client_secret/app_access_token | 源字符串作为明文商户秘密密封，不交换或刷新 | merchant-config |
| mail_contact.unsubscribe_token | 源业务退订能力密封，不作为登录材料 | business-capability |

新AAD绑定精确目标表、目标列和保留的源ID，purpose与现有protected/uniquekey组件一致。返回参数为二进制信封，匹配这12字段的LONGBLOB设计；SEO非secret设置的原文存于信封内，不能直接把源文本写入该列。空字符串生成有效的空明文信封；可NULL字段的SQL NULL继续绑定nil，不混同空信封。任一字段失败返回空BoundRow/固定ErrProtection，不返回部分密封结果或底层带值错误。

普通Convert仍对所有非NULL保护字段返回ErrProtection。新入口不会密封未知protected_string，全部protected_exact_json仍hold（包括非NULL的PayPal metadata），不能借此绕过历史认证能力清理或payload内容审查。必填旧认证账号/密码/会话/验证令牌仍在排除设计，不导入。新入口无数据库、环境、HTTP或日志副作用，也没有接入旧目录导入草稿。

新Ring由调用方提供，组件不写密钥文件或生成无法恢复的生产密钥。暂时明文字节缓冲使用后clear，但Go字符串、源JSON和密码学内部副本不保证物理内存擦除，不能声称已完成内存销毁。调用方不得打印SourceRow、BoundRow、密钥或密文；正式导入还须密钥备份/丢失策略、唯一登记、事务、恢复和读取授权。

## 验证与限制

- Go人工测试覆盖12字段、SEO is_secret分流、空/NULL、原文Unicode/空格、缺密钥/非法密文不返回半行，以及未知JSON继续拒绝。legacysecret测试覆盖严格编码、错误配置、逐字节密文篡改和非UTF-8。
- `test-legacy-secret-interop.cjs`复用既有受限VM/AST提取器执行真实四个旧TypeScript加密函数，以空串、ASCII、中文/emoji/NULL字节及长文本生成16个人工样本；Go逐项认证、通过新整行入口密封并按目标AAD解密，字节一致。fixture使用唯一随机名、Git忽略、独占创建，结束删除。不读真实.env或业务快照；默认go test明确跳过这两项实际Node互操作用例。
- 人工MySQL `TestRegistryMySQL/mapped_merchant_cipher_to_new_binary_sql_envelope`在实际mollie_configs审查表用完整转换结果绑定写入；先回滚证明不留行，再提交/读回LONGBLOB并认证原文，验证enabled=false和错列AAD拒绝。未启动Mollie、Gin、worker或调用支付。记录见mysql-type-fixture-report.json；72表示71项SQL检查加1个Go测试套件，不是72条真实数据迁移。
- 原有真实离线可读性审计再次通过：3个非空旧密文、3个空字符串、3个NULL及33人工断言不变。该脚本仍是Node基线；没有宣称3个真实值已走本次Go转换或已写入MySQL。SEO/退订等空表无真实样本。

### 真实配置的Go内存转换补充

`audit-go-merchant-config.cjs`现另行读取固定hash的Git忽略快照和显式忽略.env，选取mail_account/seo_provider/seo_setting/webhook_endpoint/mollie_config五种配置表。源字段必须与显式计划完全一致；拒绝扩展到含金额/整数/浮点的表，避免Node JSON.parse作为通用数值快照适配器。旧v1中JSON列出现null直接拒绝，不能猜SQL NULL类别；非JSON标量的null可以作为SQL NULL。它不生成v2快照、不导入数据库或触发provider。

每个实际字符串由既有受限VM中的真实旧TypeScript函数独立解密，随后将显式选择的源域密钥、配置行和期望原文通过匿名stdin管道送入Go测试程序。私有材料不放命令行参数、环境变量、临时文件或报告。由于go test命令不转发stdin，脚本先在Git忽略目录编译唯一随机名的测试可执行文件，再直接运行，结束删除；二进制本身不包含真实秘密。Go临时随机生成新32字节Ring，仅在内存执行完整ConfigSecrets.Convert，对每个信封按目标表/列/原ID解密并与Node预期逐字节相等，另验证错误行AAD全部拒绝。

结果见[go-merchant-config-report.json](go-merchant-config-report.json)：3条真实配置行、6字段（3非空/3空）、6次错行拒绝通过；SEO provider/setting源表为空，不能算真实样本。相比前一节Node基线，现已证明这3个真实旧密文走过Go整行转换；**仍没有真实新密文持久化、MySQL真实行导入或可恢复生产密钥**。既有PayPal三个NULL不在这个五表入口的真实演练内，仍由独立原基线计量。

报告只保存计数、表名、固定快照hash和实现hash，不保存源ID、原文/密文、.env hash或密钥指纹。失败仅返回固定阶段和固定白名单原因，不转发子进程输出。可清零的输入字节和临时密钥尽量clear，但JS/Go字符串及进程内部复制不能保证物理擦除。子进程超时/失败不等于成功，脚本在finally清理可执行文件；没有后台进程或定时任务。

```powershell
node retail/scripts/test-legacy-secret-interop.cjs
node retail/scripts/audit-protected-config.cjs --bundle logs/gin-migration/source-20261026.json --env medusa-store/apps/backend/.env --check
node retail/scripts/audit-go-merchant-config.cjs --bundle logs/gin-migration/source-20261026.json --env medusa-store/apps/backend/.env
```

实际MySQL测试沿用[唯一登记隔离流程](mysql-unique-key-registry.md)编译测试二进制并执行test-mysql-types.py；单核、768MiB、禁网、无发布端口，结束核对所有权删除容器。真实一致v2快照、真实新信封持久化演练、受保护JSON转换、可恢复密钥配置和生产读取权限仍缺，T006/T018不验收。
