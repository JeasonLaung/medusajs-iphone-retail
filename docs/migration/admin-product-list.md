# 后台商品列表首批实现

2026-10-28，`internal/catalog/admin_list.go` 替换 AdminProducts 原 payload 解码列表。
仅查询 ID、title、handle、status、thumbnail、collection_id、UTC 创建/更新时间和
未软删除变体数量九项，不读取或返回 payload、metadata、SKU 私密数据。
当前身份中间件仅允许启用的新 admin 账号；仓库/供应商等权限范围仍待实现。

GET `/api/admin/products` 接受单值 q、status、collection_id、sort、limit、offset。
默认20条，上限100，offset上限100000；重复、空值、未知过滤、未知排序、无效状态、
损坏编码与越界参数返回400 `catalog.invalid_query`。搜索标题/handle时将 `%`、`_`、
`!` 作为文字转义，查询值参数绑定，排序由固定白名单选择并追加 ID 稳定次序。
软删除商品不返回，count 与分页使用同一只读 REPEATABLE READ 事务。
SQL失败返回固定503，不伪装为空列表，也不暴露SQL错误。

React 新增按需加载的 `/products` 页面：Ant Form/Input/Select/Button/Table/Tag/Empty，
搜索、状态、排序、分页与 URL 同步；筛选重置页码、刷新/后退按 URL 重新加载。
请求变化取消旧响应，401退出失效登录，失败用独立 Alert/重试，无效参数可重置。
客户端验证列表结构并投影，不把多余字段保留到状态；不缓存商品数据或凭据。
目前没有编辑、批量操作或详情跳转，避免调用尚未完成的旧详情/更新草稿。

`go test ./...` / `go vet ./...` 覆盖输入边界和文字通配符转换。
[真实浏览器报告](admin-live-report.json)另覆盖实际 Gin + MySQL + Redis：人工商品
包含故意私密及过期 payload，列表只返回真实列，软删不计数、未删变体数正确，
重复/未知过滤拒绝，含 `%_!` 的标题可精确搜索，越界分页为数组空态。
Ant 表格实际搜索、刷新、空结果、重置、无效 URL 恢复与三宽度无页面溢出均验证。
浏览器测试使用0001草稿表和人工数据，不展示真实商品或私密数据。

另扩展[真实目录SQL演练](catalog-mysql-rehearsal-report.json)：在完整审查版表结构、
外键开启及23,559行目录/父依赖提交后，直接分发真实Gin列表handler，遍历30个未删
商品，分页无重读遗漏；每项标题/handle/状态/未删变体数与独立SQL一致，实际handle
和status组合过滤命中。查询前后45张业务表和5张辅助表的完整指纹一致，随后精确
重放继续通过。响应仅留在隔离进程内，日志及报告仅记录计数。此路径没有认证
中间件和浏览器，因此身份/浏览器证据仍由上述人工新账号链路独立覆盖，不声称
真实目录加完整登录的联合验收。375项全域引用缺口仍保留失败。

读取字段有意取自现有草稿和审查版共同的权威列；这不是schema迁移完成证明。
公开接口、详情、更新仍有 [catalog-contract](catalog-contract.md) 登记的草稿缺陷，
尚未被本批修复。完整数据结构、全部关联/价格/库存、角色对象范围、商品创建编辑、
并发审计及Astro前台仍待实现。T020/T021/T023不验收，前台没有改动。
