# DHL 物流演示迁移

2026-11-01。Astro 公开 `/{countryCode}/shipping-demo` 与只读 `/api/store/shipping-demo/dhl` 已接 Gin，主预览已受控更新。账户 `/{countryCode}/account/shipping-demo` 沿用既有新客户观察和登录边界：有客户才显示演示，无客户走登录；主环境客户门禁继续关闭，因此不称已完成已登录账户联合。

来源为 `medusa-store/apps/backend/src/lib/dhl-official-demo.ts`、原 `modules/order/components/dhl-official-tracking/index.tsx`、公开/账户 shipping-demo 页面及 `shared/dhl-demo-copy.json`。保留固定官方样例、公开 demo-key、15秒超时、禁止重定向、30秒缓存及并发共享、生产环境404、失败502和no-store。查询参数不能改变样例单号，不用于真实订单物流；真实订单包裹状态继续使用既有独立流程。Gin保留原shipment字段，失败不暴露上游细节。

前台保留原结构、类名、加载/失败/刷新、刷新失败时已有结果，以及原始事件时区展示。中英文JSON逐字节复制；复用StorefrontTheme，按当前Ant Design 5将Alert title适配为message、Timeline content适配为children。公开页保留Shipping demo标题及noindex/nofollow。账户组件仅新增默认false的演示分支，不新增来源不存在的账户菜单项。

桥接只访问服务端固定Gin地址/路径，不转发cookies、Origin、客户或管理员凭据；校验固定样例、官方来源、时间戳及事件形状，响应有1MiB上限。共享publicJSON默认5秒保持不变，仅该查询放宽至17秒，以容纳Gin原15秒查询。专有测试实际等待超过5秒证明不会被普通读取超时提前中止。

## 实际验收

[本轮联合报告](shipping-demo-native-report.json)记录原Next8000与实际Astro9300/Gin9100/持久99迁移MySQL。中英×390/1440共8组：官方固定样例12事件、显示文字对照、刷新、网络中断后已有结果保留、恢复重试、官方文档链接、noindex及无横向溢出。无合成API响应；错误重试通过浏览器中断请求验证。外部HTTPS图片/字体被阻止，不称远程资产或全站逐像素验收。页面异常和业务写请求均0。命令：`node --experimental-strip-types retail/admin/tests/shipping-demo.native.mjs`。

前台全205项测试通过，包含原账户/购物车真实HTTP传输；首次沙箱运行两项监听失败，允许本地网络后全部通过，未修改这些测试。Astro check 448文件、0错误、0警告、17提示。此前DHL包race/server/vet及实际官方样例检查见[源码阶段历史证据](shipping-demo-source-report.json)；该报告的未接线标志保留当时事实，本报告记录后续接通状态。

## 主预览更新与数据保护

Go可再生成构建缓存原占约22GiB，清理后空间从约600MiB恢复到约23GiB；少量root旧缓存未动，无需更改其权限，所有验收证据、运行二进制和业务数据保留。之后冻结owner42fb2f0d44d4完成全部原23组真实隔离SQL回归，专有MySQL/Redis资源清理完成；与上次d87bbfe447dd后端仅差DHL新增包和路由注册，99迁移字节一致。

`retail/scripts/refresh-local-preview.py --owner 42fb2f0d44d4 --previous-owner d87bbfe447dd`从已通过回归的冻结源码构建，核对当前后端未变、迁移不变、日志/测试binary SHA、主健康及运行binary后，先停旧唯一Gin再替换并启动。运行PID372，管理仍使用`preview-api.py`。旧binary和原cutover事实保存在`log/gin-preview-mysql/binary-refresh-42fb2f0d44d4/`；当前cutover事实更新运行binary SHA并记录owner，原历史切换报告不改写。脚本有失败恢复路径，本轮未触发，不称实际反向恢复演练完成。

更新前后主库只读对账通过：293表中291非journal与fresh演练一致，268原表不变，99校验和、213外键无孤儿，保留共享86库全部指纹仍一致；不改schema、配置、门禁，不启动worker或第二个同库后端。9001健康/Admin、Gin健康、React Admin、Astro商店/物流/账户路由正常；商店初始HTML仍30款，无效国家/语言404，近期非空服务日志无panic/fatal或意外5xx。现有购物/评价等关闭门禁保持。

后续仍需已登录新客户账户物流演示联合、完整SEO/地区交互和C01其余状态，及C02–C11完整账户、交易、履约、售后、回收、真实测试渠道及邮件联合验收。整个迁移不标完成。

账户关闭门禁另经实际Chrome中英×390/1440四组验证：原账户错误提示可见、未渲染物流面板且DHL查询0、页面异常0/无溢出。报告见联合报告account_closed_gate_browser；不替代已登录客户验收。
