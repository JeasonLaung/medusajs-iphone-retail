# React 后台组件与页面承接契约

日期：2026-10-27。落实用户指定的 Ant Design + Apache ECharts；属于 T007/T023/T027/T038/T048/T054—T059 的设计输入，尚无已验收的新后台业务页。2026-10-28已新增[Ant登录/账号局部实现](admin-login-ui.md)及人工HTTP浏览器证据，尚缺真实端到端和完整业务页。

## 参考范围

已检查 `../../iphone0902/backend/web/src/App.tsx`、`pages/Dashboard.tsx`、`components/AccountSettingsModal.tsx` 与 `package.json`。参考项目确实使用 React、Ant Design、React Router 与 ECharts：Layout/Menu 管理导航，Tabs 管理页签，Drawer 提供移动菜单，ConfigProvider/App 提供主题和反馈上下文；图表按需注册、ResizeObserver resize、卸载 dispose。

这些结构适合复用。其询盘、爬虫、批发库存等业务不直接移植；登录协议、菜单权限和统计数据改为本项目 Gin 契约。参考项目的 `#409eff` 主色及独立图表色板不能覆盖本项目主题。安装版本在 T023 实施时核实兼容性并锁定，不能从参考项目版本范围推定已安装或已验证。

## 组件选择

| 场景 | 优先组件 | 保留的业务约束 |
| --- | --- | --- |
| 框架、导航、页签 | Layout、Menu、Breadcrumb、Tabs、Drawer、Dropdown | 菜单、页面、动作分别鉴权；详情入口刷新有效；隐藏菜单不替代服务端授权 |
| 查询、列表、批量操作 | Form、Input/Search、Select、DatePicker/RangePicker、Table、Pagination | 服务端分页/过滤/排序与 URL 同步；稳定 rowKey；当前数据范围 count；跨页选择必须明确对象集合 |
| 业务编辑 | Form、Input、Select、Cascader/TreeSelect、Switch、Radio、Checkbox、InputNumber | Form.Item 显示字段错误；SN/IMEI/EID/19 位编号始终文本；精确金额用 stringMode 或受控字符串，不经 JS 浮点计算 |
| 明细、状态与过程 | Descriptions、Tag、Badge、Timeline、Steps、Typography | 状态文案与图标一起表达；可执行动作来自权限及实际状态，不按显示颜色猜业务 |
| 编辑/命令确认 | Drawer、Modal、Popconfirm、Button | 普通编辑用表单；付款/退款/发货等显示对象、数量和金额；pending 防重复且沿用同一操作键；冲突提示重新读取 |
| 图片、照片与文件 | Upload、Image、Progress、Table | 类型/大小/数量在服务端再验；私有图片授权；撤销上传不删除已被业务引用的文件；XLSX 解析预览保留行号与整批校验 |
| 加载与反馈 | Skeleton、Spin、Empty、Alert、Result、App 的 message/notification/modal | 初次加载、空结果、请求失败、无权限分别显示；失败重试不变成创建第二笔操作；反馈关联 request_id |
| 数据卡与图表 | Statistic、Typography + ECharts | 单项数字用 Statistic；趋势、分布等可视化全部使用 ECharts；口径和权限由后端确定 |

业务包装组件只封装重复规则，例如受保护照片、精确金额字段、带权限的命令、领域筛选器和 ECharts 容器。扫码/相机、OCR、富文本与 XLSX 预览按实际需求保留专用能力，外围使用 Ant Design；不为“全部 Ant”删掉现有交互。

## 自定义页面逐项去向

下列路径相对 `medusa-store/apps/backend/src/admin/routes/`，每一行对应已盘点的一个 `page.tsx`。原生商品、订单、客户、库存及设置页面还需 T002 单独覆盖；这 23 页不是全部后台。

| 当前页面目录 | React 页面承接及主要组件 | 任务 |
| --- | --- | --- |
| device-scan | 保留权限决定的入口重定向：带 order_id 到订单；仓库按权限到入库/退回，其他到检索；保留查询参数 | T048 |
| device-scan/intake | 单台/批量入库：Form、Upload、Table、Drawer，保留扫码、OCR、校验预览 | T041、T048 |
| device-scan/inspection | 兼容重定向到 returned，保留 device_id；检测表单在设备工作台承接 | T041、T048 |
| device-scan/lookup | 设备检索与明细：Input.Search、Table、Descriptions、Timeline | T040、T048 |
| device-scan/orders | 兼容重定向到 orders 或带 order_id 的订单详情；配货/打包/履约在订单区域承接 | T043、T044、T048 |
| device-scan/prices | 回收报价目录 BuybackPriceCatalog：Table、Form、精确金额输入 | T050、T053 |
| device-scan/recycled | 回收设备工作台：Table、Drawer、Form、Image | T041、T052、T053 |
| device-scan/requests | 回收申请 BuybackApplications：Table、Tabs、Drawer、Timeline、状态动作确认 | T051—T053 |
| device-scan/returned | 退回设备及检测：Table、Form、Image、Timeline | T045、T046、T048 |
| device-scan/sources | 来源/供应商及供量：Table、Form、Drawer，按来源裁剪成本字段 | T042、T048 |
| mail | 邮件工作台：Layout、Tabs、List/Table、Drawer、Upload；保留编辑器、附件和队列状态 | T055、T056 |
| seo | SEO 设置及页面信息：Tabs、Form、Table、任务状态反馈 | T058 |
| seo/citation-monitor | 引用监测：Table、Form、Drawer、ECharts，显示真实任务结果 | T058 |
| seo/crawler-logs | 爬虫日志：Table、筛选、Descriptions，权限与脱敏 | T058 |
| seo/providers | SEO/provider 配置：Form、Table、Modal；密钥只写不回显 | T058 |
| seo/traffic-sources | 流量来源：日期筛选、Table、Statistic、ECharts | T058 |
| settings/buy-policies | 购买政策：Form、Table、Drawer，保留原字段与规则校验 | T058 |
| settings/customer-service | 客服配置：Form、Select、状态反馈 | T058 |
| settings/mailboxes | 邮箱配置：Form、Table、Drawer；发送/验连等动作分别授权 | T055、T056 |
| settings/mollie | 支付配置：Form、Alert、Descriptions，区分环境/启用状态/商户验证 | T035、T058 |
| settings/warehouse-access | 仓库权限：Table、Select/TreeSelect、Form；区分所有仓库与空范围 | T015、T048 |
| settings/webhook-notifications | 通知配置：Form、Table、Modal，保留任务结果及错误反馈 | T057 |
| shipping-demo | DHL 演示：Form、Steps、Table；明确原有演示状态 | T037 |

## 六个 widgets 的承接

源目录 `medusa-store/apps/backend/src/admin/widgets/`：

| 当前文件 | 新去向 |
| --- | --- |
| order-devices.tsx | React 订单详情设备区域，复用配货/解配命令与权限；Table、Drawer、Descriptions |
| order-after-sales.tsx | 订单售后区域，复用退回/换机/维修/退款流程；Table、Timeline、Modal |
| order-shipping.tsx | 订单履约与物流区域，复用分批发货及取消约束；Table、Steps、Descriptions |
| product-reviews.tsx | 商品评价区域，保留查询、审核及相关权限；Table、Rate、Drawer |
| mail-topbar.tsx | 新后台顶部邮件入口与状态；Badge、Popover/Drawer，读取失败不当零未读 |
| selectable-messages.tsx | topbar 挂载的全局消息文本选择修复：当前拦截 Sonner pointerdown 以允许复制；新 Ant 反馈保留可选中/复制错误文本，不照搬 Sonner 事件补丁 |

编译时改写的仓库菜单、库存排序、订单履约选择、Analytics 集成要进入新路由/组件，不能仅迁移这些 widgets。只读访问角色不得因使用通用 Drawer 自动获得保存、上传或危险操作。

## 主题、响应式与状态

通过 ConfigProvider 与共享语义 token 应用仓库 AGENTS.md 的品牌 `#1677FF`、强品牌 `#0958D9`、底色 `#F8F8F8`、白色表面及功能色。保留 Ant 组件的受控状态、键盘操作和清晰焦点；后台默认字体与现有运营界面保持一致，不强制套前台标题字体。容器优先背景及间距分组，避免每层 Card 都加边框/阴影。

中英组件 locale 与业务翻译同时切换。窄屏导航用 Drawer；大表保留必要列并提供局部横向滚动；抽屉宽度受 viewport 约束。页签切换保持筛选上下文，未提交表单关闭时提示；退出/撤权清理受保护页签和缓存。没有明确需求时不把整个业务对象写入 localStorage。

## ECharts 生命周期与数据

- 按需注册图表、坐标系和渲染器；具体封装及版本在实施时确定，不额外引入另一套图表库。
- 单个容器管理单个实例；容器有尺寸后初始化，ResizeObserver 处理侧栏、Drawer、Tabs 变化；隐藏页签重显时 resize；卸载清理 observer、事件及 dispose，React StrictMode 重挂载不得重复实例。
- 数据请求区分 loading、success-empty、success-data、error；请求失败不得用零替换真实值。多图部分失败按图显示错误；过期请求不得覆盖新过滤结果。
- 金额字符串保留精确值用于标签/tooltip；坐标绘制需要 number 时仅做显示转换，并保留币种、单位、时区和原值。不同币种无明确汇率口径不能叠加为一个销售额。
- 图表 tooltip 使用安全文本/转义，不拼接不可信产品名或 provider 内容为 HTML。提供可读标题、数据摘要或表格入口，减弱动态效果遵循 reduced motion。
- 统计契约见 [Analytics](analytics-contract.md)；图形相似不等于数字口径相同。除统计页，还覆盖 SEO 流量、引用监测等确有图表的页面。

## 验收点

T023 样板需验证：路由刷新/前进后退、桌面与移动导航、页签关闭与表单保护、中英、表格分页及筛选、表单字段错误、401/403/409/网络失败反馈。T027 验证重复挂载无实例泄漏、容器缩放、快速切换过滤、空数据与失败区别、币种/日期口径及权限裁剪。各领域随后核对允许与拒绝操作，不用静态菜单、截图或假数据图表作为业务完成证明。
