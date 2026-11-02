# 仓库读取503与请求取消诊断

2026-11-02，上轮实际仓库履约虽然通过，但权限、配货、释放、发货等 GET 留下503，旧日志无法区分客户端取消和有效请求故障。本批增加访问日志 `request_context` 固定枚举 `active`、`canceled`、`deadline_exceeded`，按请求完成时 `Context.Err()` 分类。它不打印错误内容、取消原因、参数、查询串、请求头或正文，不更改响应状态或业务重试规则；取消请求仍保留原503记录，不能通过改成成功掩盖问题。

单元验证覆盖有效、取消及过期上下文，保留原503、服务端请求ID和脱敏路由。CancelCause 的内容不覆盖 Context.Err 分类。现有边界测试继续限定日志字段白名单、拒绝泄露私有请求资料；platform/server 测试及 vet 通过。

新增 `ACCOUNT_TEST_API_BINARY` 可选择 `logs/retail-native` 内已编译的专属 API，检查路径与.exe文件。运行器对实际运行文件计算SHA，主服务保持原端口。实际执行：

```powershell
$env:ACCOUNT_TEST_API_BINARY = Join-Path (Get-Location) 'logs/retail-native/retail-api-diagnostics.exe'
try { python retail/scripts/test-local-customer-account.py --warehouse-from-checkpoint=790ce0fc9e71 }
finally { Remove-Item Env:ACCOUNT_TEST_API_BINARY }
```

执行 `c1e78ed8c83f` 继续原已付款订单的完整仓库子流程，通过原命令丢响应恢复、手机员工范围限制及客户物流/设备/通知。此次运行器及后置SQL直接通过，无须重新支付。后台全部50条503的请求上下文均 canceled；有效后台请求没有503。另2条 active503仅是本副本显式关闭的客户补款、评价读取。React effect cleanup AbortController 与页面重载会取消过时请求，此次证据支持该原因，不能倒推旧日志每一条503，也不能把所有 canceled 请求都宣称为 StrictMode 所致。

库存50→49、预留2→1，订单金额/身份、支付/捕获/交易/归属等保护指纹不变；源12表前后均0，专属进程/副本/授权及10条Redis键清理。见 [实际执行和分类报告](warehouse-request-cancellation-report.json)。前台/后台交互和样式未改，不为每个读取添加重试，也未启动新支付、退款或邮件任务。

验证后用现有 `scripts/start-retail.ps1` 受控更新本机三端，使后续日志可直接分类；未迁移数据库、未修改共享Redis或端口。旧9001/8000仍按原切换保持关闭，当前原生入口为9100/9200/9300。本批解决已复现的仓库读取诊断缺口，C01—C11及真实渠道/邮件等剩余交付保持开放。
