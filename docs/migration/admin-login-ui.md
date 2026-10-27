# Ant Design 登录及账号页面

2026-10-28，独立新增`retail/admin`。复用参考iphone0902的React/Ant布局方向，不复制其业务、认证协议或颜色。现有Medusa后台和Next前台不修改。新页面直接调用Gin接口，不安装Medusa SDK。

锁定React19.2.8、Ant Design5.29.3、React Router7.18.3、Vite8.2.2及TypeScript6.0.2；实际npm元数据和安装版本核对，npm lockfile提交。Ant5/React19兼容层使用[官方patch](https://github.com/ant-design/v5-patch-for-react-19)1.0.3，入口首先导入；浏览器测试检查没有兼容告警。Ant组件使用参考[官方介绍](https://ant.design/docs/react/introduce/)，构建边界参考[Vite文档](https://vite.dev/guide/)。未引入ECharts，因为此批没有图表，后续图表固定使用ECharts。

Form/Input.Password/Button完成登录，ConfigProvider/App及locale管理主题和中英，Layout/Menu/Drawer提供账号页与移动导航，Descriptions/Tag显示当前账号，Alert/Spin/Result区分连接中、匿名、请求失败及未知页面。主题来自本项目规定的brand/success/warning/error及白/浅灰表面；无装饰渐变/重阴影。只显示真实实现的“我的账号”，不以空业务菜单冒充商城后台。

`/login`在提交时先POST一次性challenge，再带X-CSRF-Token登录，不自动重试写请求；失败后清空密码，下次获取新挑战。提交期禁用表单且同步ref阻止重复提交。`/account`首次读取me、刷新重读；只有401进入登录态，503/网络错误保留独立错误及显式重连。语言是唯一localStorage偏好；不将账号资料、密码、session或CSRF写入local/session storage。会话cookie由浏览器HttpOnly保存，退出时从可读CSRF cookie读取header并交服务端校验，成功才清除本地登录状态；退出失败保留账号且可以重试。重新聚焦/页面可见时复核me，401转重新登录。

API客户端固定同源`/api/admin/auth`，10秒请求超时，拒绝重定向、禁止缓存、显式same-origin credentials，不接受任意后端URL。仅投影验证过的user字段；错误按稳定code本地翻译，不显示原始后台message/HTML或异常。合法request_id可显示供追踪。会话已过期、错误凭据、Origin/CSRF、限流和依赖失败分别反馈。

开发9200、独占测试19200，均strictPort，测试不复用已有服务。代理只指向新Gin9100并保留请求Host/Origin；示例ADMIN_ORIGIN已统一为`http://127.0.0.1:9200`，切换localhost必须同步显式配置。构建只写新应用dist，不触碰运行中旧服务输出。

[浏览器实证](admin-ui-report.json)：TypeScript及生产build通过，9项真实浏览器测试使用人工HTTP响应，覆盖登录/刷新/退出、私有资料不进storage、错误凭据清密码并重取挑战、依赖失败不伪装成匿名、退出失败保留登录并重试成功，以及390/768/1440px三宽度的中英登录/账号状态与移动Drawer。新增商品列表依赖失败与空态区分、恢复重试及401回登录，以及商品详情503恢复/404/描述HTML纯文本验证。语言Select用键盘打开并点击选项验证，检查无横向溢出及无运行时异常；九张截图留在忽略目录，报告仅保存摘要。已目视检查中文桌面登录与英文手机账号页面。

本次七项UI浏览器测试采用显式本机Edge覆盖路径，不声称Playwright默认Chromium或跨浏览器通过；接口响应由测试拦截，不连接真实Gin/MySQL/Redis。另有[真实浏览器链路验证](admin-live-browser.md)，已串联实际Gin进程、MySQL和Redis，验证回环HTTP下cookie/Origin/代理。生产bundle当前约889kB（gzip283kB），保留构建体积告警，后续领域路由须按需拆分。生产HTTPS/Secure、完整后台外壳、页签/业务菜单权限、客户认证、商品页或Astro仍缺。T014/T023/T024仍未验收。
