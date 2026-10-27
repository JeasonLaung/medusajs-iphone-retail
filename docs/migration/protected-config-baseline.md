# 商户配置密钥与离线可读性

T006/T018 的部分证据。脚本 `retail/scripts/audit-protected-config.cjs` 读取固定、Git忽略的业务快照及显式指定的旧backend/.env，仅提取四域密钥与COOKIE_SECRET；不启动app、访问数据库、发邮件、刷新token或调用支付/通知/SEO服务。不打印或保存明文、密文、密钥指纹或行ID。聚合报告见 [protected-config-report.json](protected-config-report.json)。

## 字段与源格式

| 字段 | 旧格式/密钥优先级 | 新持久化设计 |
| --- | --- | --- |
| mail_account.api_key/webhook_secret | AES-256-GCM，MAIL_ENCRYPTION_KEY或COOKIE_SECRET | 独立版本化认证信封；不复用登录cookie密钥作新业务密钥 |
| seo_provider.api_key，seo_setting.value在is_secret=true时 | SEO_ENCRYPTION_KEY或COOKIE_SECRET | secret值解密后密封；is_secret=false维持原文本语义，不能盲目当旧密文解密 |
| webhook_endpoint.url/secret/token | WEBHOOK_ENCRYPTION_KEY或COOKIE_SECRET | URL也可能带capability，三字段均保护，不试发验证 |
| mollie_config.api_key | MOLLIE_ENCRYPTION_KEY或COOKIE_SECRET | 解密后新信封，环境/启用状态保留但演练不启动provider |
| paypal_connection.auth_code/seller_client_secret/app_access_token | 源provider字符串，不假设与上述AES格式相同 | 字符串直接作为受保护商户秘密密封；不交换或刷新，NULL/空保留 |
| mail_contact.unsubscribe_token | 明文业务退订能力，不是客户登录token | 受保护存储及单独查询索引；不导入客户/员工登录能力 |

四个源加密实现均使用SHA-256(配置字符串)得到32字节AES密钥，12字节随机IV，保存base64(iv).base64(tag).base64(ciphertext)。解密空字符串直接返回空；缺配置或默认supersecret在需要密钥时拒绝。域密钥名称分开，但若都回退COOKIE_SECRET，则实际可能同密钥；旧信封没有行/列/域的AAD绑定，不能宣称有密码学域隔离。新密钥及信封必须独立设计，不能直接复制旧密文到新库后依赖旧COOKIE_SECRET。

脚本只从真实TypeScript AST提取key/encrypt/decrypt及必要错误函数，VM只开放node:crypto和固定env，框架错误类为替身；不执行源码中其他查询函数。dotenv仅parse，不改宿主process.env；报告保存5个非秘密来源文件哈希，不保存.env哈希。当前只核对显式选择文件，不证明运行中的进程或部署环境使用相同配置。

## 本机固定快照结果

3个非空旧密文可解密：mail API key、通知URL、Mollie API key。3个空字段为邮件webhook secret、通知secret/token；PayPal三个受保护字段为NULL。SEO配置和退订联系人表为空，不能当作该域真实数据通过。

每个非空值在内存使用新随机32字节临时密钥、12字节IV做AES-GCM往返，AAD包含版本/用途/表/列/旧ID，校验字节相同，并拒绝错误AAD。临时密钥和buffer使用后清零，不保存新密文；JS字符串及VM不保证物理内存完全擦除，故不将其描述为内存销毁证明。这是转换可行性演练，**没有可恢复的生产新信封或导入器实现**。

33个人工断言覆盖四域真实函数的往返、随机IV、空解密、错误密钥、缺密钥、COOKIE回退、默认密钥拒绝、密文篡改，以及独立AAD往返/错上下文拒绝。未验证provider凭据是否有效、商户权限、远端附件可下载性，也没有证明密钥轮换或数据库事务。

```powershell
node retail/scripts/audit-protected-config.cjs --bundle logs/gin-migration/source-20261026.json --env medusa-store/apps/backend/.env --check
```

首次或审查后去掉--check生成聚合报告；--check重新执行但不覆盖报告，源哈希/计数/结果变化失败。错误输出固定，不转发可能包含秘密的底层异常。输入快照/.env必须被Git忽略，快照SHA固定。无需生产商户凭据，不读取或迁移旧登录密码/会话/重置/验证令牌。

目标[版本化信封组件](protected-envelope-contract.md)现已实现版本/key ID/AAD标准，人工字节往返、MySQL持久化及轮换回滚通过；新[12字段Go转换入口](merchant-secret-conversion.md)已使用四个真实旧加密函数生成的16个人工样本证明互操作，并在人工MySQL验证整行二进制持久化/回滚。该组件尚未读取或转换本基线中的真实旧密文。仍须独立密钥保管与备份/轮换/丢失策略、真实MySQL导入演练、读取授权/错误脱敏，以及新实例上不可回放外部任务的证据。2封旧注册验证邮件必须禁止重放且清理认证capability；本脚本不扫描邮件正文或导入任何旧账号材料。T006和T018仍未验收。
