# 移动凭证、模板下载、退订与邮件回调

机器设计见 `capability-entrypoint-contract.json`，11 个入口、28 个源码文件 SHA-256。生成器只读源码，不启动业务、不发邮件、不校验真实客户凭据。T004/T005/T007 仍未验收。

## 移动入库

原页面 `/static/mobile-intake/index.html?lang=zh|en#TOKEN` 位于 backend/static/mobile-intake，不能从 Next/Astro 页面清单推定已覆盖。页面保留中英、相机/条码及图片扫描、SKU 搜索分页、照片、手工身份、费用/备注、批次收据、统计和撤销交互；最终承接至新 React/Astro，不保留 Medusa 调用。页面本身公开静态壳，业务请求必须通过能力授权。

当前脚本从 location.hash 读取令牌，fetch 使用 x-intake-token、credentials:omit、no-store、30秒超时。令牌不放 query，不写日志或缓存；原页面 no-referrer、noindex,nofollow 保留。新签发使用新密钥和新员工 ID，旧 JWT/员工凭证不继承。

旧 HMAC payload 为 purpose/actor/source/category/expires_at/nonce，默认两小时、最长2048字符，签名恒定时间比较，到期值等于当前时间即失效。源码允许缺 category 回退 sale；新签发总带类别，旧 token 不恢复。纯验签只检查有限字段形状，Gin 应严格验证 string/长度、到期范围、nonce 和签发版本，不能把成功验签等同于完整身份授权。

每个请求重新核对当前员工状态、设备动作、来源和仓库权限；source 必须 self，仓库 recycled 限制由现有 warehouse guard 承接。POST body 不接受 actor/source/category 覆盖；服务端从签名派生。所有设备领域检查、审计及数量库存独立规则继续适用。

DELETE 特别要求：设备相同来源/类别，pending/inspection/available，且有**同一 actor、同一 nonce** 的 intake 事件，另无 reserved/packed/shipped/returned assignment 历史。不能用“本仓设备”替代该会话创建证据。nonce 当前不是一次性使用，链接可连续入多台；业务写和重复删除通过新 MySQL 幂等账本处理，不能把 nonce 全局消费一次破坏批量入库。

照片1—8张，单张声明 base64 上限4194304，解码最多3MiB，真实JPEG/PNG/WebP头与MIME一致；旧HTTP请求限34mb。源 `device-photos.ts` 明确把**设备照片设为 public**。回收照片/邮件附件仍 private，不能把两种存储误合并或因迁移统一挂 static。新设备照片公开范围须随实际 DTO/资源权限验证，不用“不是回收照片”推断其中没有敏感内容。

## 模板下载

原 GET `/admin/devices/import-template?locale=…` 遍历全部变体，每页1000，SKU/ID稳定排序；返回 Office XLSX MIME、固定附件文件名及 no-store。供应商拒绝；仓库和动作权限仍经过中间件。下载不产生业务写入。

现有模板含中英表头、身份字段文本格式、200行输入区、SKU source/veryHidden choices、参数搜索、校验和计算公式，不能用简单CSV替代。后续 Go生成器/React导入预览需保持这些能力，并验证长IMEI/EID不被科学计数法截断、SKU搜索/选项及Excel/WPS行为。当前只读源码，未生成/渲染新模板，不能宣称已兼容。

## 退订与同意

GET `/mail/unsubscribe/{token}` 校验64小写hex语法并展示中英确认，不修改订阅；邮件安全扫描器读取链接不能退订。POST查找匹配contact后置subscribed=false，成功200、重复成功仍200、无效404；不清除suppressed、不重订阅、不发邮件。

退订token是业务同意能力，应保护迁移，区别于排除的登录/重置/验证token。外部邮件收件人不需要商店登录或普通登录CSRF会话；使用能力表单及明确Origin策略。确认页保留CSP default-src none、form-action self、frame-ancestors none及no-store，路径日志必须替换token为路由模式。

## Resend回调及持久接受

原POST `/mail/webhooks/{account_id}` 保留最多1mb rawBody，用该邮箱保存的webhook secret验签；svix-id/timestamp/signature映射到已安装Resend的Standard Webhooks验证器。未验签不能执行业务，浏览器CSRF不替代provider签名。

本地Resend 6.33.0实际调用standardwebhooks 1.1.1；已读取其编译产物。该验证器签名输入为`event_id.timestamp.raw_payload`，v1 HMAC-SHA256，支持空格分隔多签名，时钟容差±300秒，边界300秒接受、301秒拒绝。JSON重新序列化/空格变化会破坏签名，不能先bind再marshal验签。新Go适配需按实际provider测试并锁定规则。

发现旧验证器用parseInt解析timestamp，合法秒数后带字符串也会接受。固定用例记录旧行为，新契约明确拒绝非规范整数；不机械继承该解析弱点。真实回调时效仍须独立时钟/HTTP集成验证。

工作流去重键为account_id:event_id，完成后记事件；收件向Resend取实际内容，to/cc/received_for匹配当前邮箱才导入。投递更新也需匹配邮箱/活动，bounce/complaint/suppressed使联系人退订并抑制。delivered不被sent降级，失败终态优先；opened/clicked只记首次时间，不把投递主状态改成opened/clicked。

新实现先在MySQL可靠保存已验签事件和处理意图，再204确认接受；后续租约worker处理失败可恢复。原同步处理失败允许provider重试，不能改成先204、仅写Redis/内存后丢事件；也不能先标已处理再下载实际收件导致失败无法恢复。收到重复事件不重复导入、发信或改变同意。历史导入期间worker及远端收件调用保持hold。

## 已运行证据与后续

`capture-capability-rules.cjs --check`有40个人工用例：移动签名/到期/类别/nonce、9组邮件乱序状态、11组已安装回调签名规则。仅执行纯函数或提取的状态函数；framework异常构造使用明确stub，回调时钟固定，使用人工密钥且不保存签名/token。

这些结果不证明新Gin权限撤销、nonce删除归属、原始HTTP body、并发去重、文件下载、XLSX或页面已验收。还需全部自定义方法到新API的完整清单、原生后台/支付插件契约及新请求/响应schemas。此批11个入口与既有调用去向有重叠，不能把数量相加宣称独立功能数。
