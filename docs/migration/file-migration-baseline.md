# 文件迁移本机基线

T006/T016/T018 的只读核对。使用固定业务快照 source-20261026.json（SHA-256 `aa18ee30d5a7dcb11efe20a5c8785abfdbe561c362db4503160b262df6a2f132`）与当前 backend/static、backend/.private-files，未复制、删除、上传文件，未抓取远端或调用邮件provider。报告 [file-snapshot-report.json](file-snapshot-report.json) 只含聚合计数及来源哈希；逐文件路径、对象关联和哈希在 Git 忽略的 logs/gin-migration/file-index.json，不入 Git。

## 真实存储与访问

| 类别 | 旧实际方式 | Gin 迁移要求 |
| --- | --- | --- |
| 商品/变体 thumbnail、image.url | URL；/static/ 在本地公开目录，其他HTTP(S)为远端 | 保留路径/图片内容及页面引用；本地与远端分开清单，不能以URL字符串存在认定文件可用 |
| device.photos | 字符串URL数组；上传工作流明确 access=public | 保留公开设备图语义，上传仍校验员工/移动凭证、真实文件头与3MiB上限；不能混入私有回收图 |
| buyback_photo.storage_key | file-local私有对象，key以private-开头；项目配置 private_upload_dir 为环境覆盖或默认 .private-files | 私有目录不挂static；继承对象与申请/条目归属，授权后读取；存储key不返回客户或后台 |
| 回收照片下载 | enclosing申请先鉴权，再按 photo_id+request_id 查记录，expired/空key拒绝；返回base64及mime_type，private,no-store | 保留当前响应和受保护访问，或在对应前台适配中明确替代；不能简单改成可访问原文件的公开URL |
| mail_message.attachments[].content | 数据库JSON中base64；下载为attachment/octet-stream及nosniff | 私有附件需和消息/邮箱范围一起授权，字节及文件名保留，内容不能被公开DTO透传 |
| 邮件无content附件 | 消息provider_id+附件id+邮箱api_key，Resend获取download_url | 没有本地文件不等于丢失；远端可下载性、密钥可解密性和受保护响应需单独验证；不能离线补造内容 |

已读取已安装file-local 2.21.2实现：public默认写当前工作目录/static，private_upload_dir若未配置也默认static。**本项目明确覆盖为 .private-files**；provider使用key的private-前缀选择目录，并做resolve路径范围检查。迁移不能只照抄provider默认值。当前审计目录由命令明确传入，尚未核对所有可能的环境覆盖，不宣称生产目录或未来配置已确认。

回收照片到期工作流仅针对已关闭且超过180天的申请，先删对象再将记录storage_key置空、状态expired；失败保留重试可能。本次审计不运行清理，不将未被字段引用的文件当垃圾删除。旧客户身份不导入，因此历史申请的照片不会因新客户邮箱相同自动授予读取权限。

## 本轮结果

- 公开目录412文件、84,412,810字节；本轮字段引用394个唯一key，18个文件未被这些字段引用，全部保留。
- 私有目录6文件、110,387字节，6个key均由回收照片记录引用；图片头与声明MIME相符、每个不超过3MiB；未发现同key同时位于公开目录。
- 商品图434次、商品/变体缩略图958次、回收照片6次，合计1,398次引用；其中1,342次本地文件存在，56次HTTP(S)远端引用未抓取。重复引用不会被计作多个物理文件。
- 固定快照 device.photos 没有照片引用，mail_message没有附件；空样本不验证设备图或内嵌/远端附件迁移成功。
- 未发现本轮范围内的缺失本地文件、无效路径或私有对象作为公开图引用。图片头检查不等于完整解码成功或视觉一致性。

数据库快照是固定时点，文件目录是当前本机；不是跨数据库和文件的一致性快照，也未冻结开发上传。脚本逐文件比较哈希前后大小与mtime，拒绝扫描期间改变的文件，但不能保证数据库引用与文件在同一个事务时点。

## 校验方法

```powershell
python retail/scripts/test-file-audit.py
python retail/scripts/audit-file-snapshot.py --bundle logs/gin-migration/source-20261026.json --public-root medusa-store/apps/backend/static --private-root medusa-store/apps/backend/.private-files --private-index logs/gin-migration/file-index.json --check
```

首次或审查变化后去掉--check生成；--check会重新哈希并与私有明细、聚合报告比较，不覆盖原报告。不读取.env或凭据；目录必须显式选择且公开/私有互不包含；拒绝链接/目录连接、跨目录key和不可移植路径。遍历失败和扫描中改变均拒绝，不把读取失败算成空目录。测试只使用临时人工文件，覆盖路径穿越、路径大小写、只读哈希、图片头及链接拒绝；操作系统拒绝创建链接时该项显式skip。

## 尚缺

前台public目录、CSS/HTML/邮件正文中的图片和任意metadata资源仍要补清单；本轮明确字段不是全站资源扫描。56次远端引用的唯一资源、可用性与保留策略尚未验证，不能绕过现有图片授权限制批量下载。邮件附件为空，不替代有附件的人工迁移样例。

T016/T018还需文件复制演练、目标哈希核对、公开路径兼容、新私有授权/过期访问/路径穿越测试、历史主体归属及新的冻结/增量流程。T006仍需密钥、全部约束默认值与语义差异对账，不能因6张照片存在而验收全文件迁移。
