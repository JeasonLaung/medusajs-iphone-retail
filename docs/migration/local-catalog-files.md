# 本机原商品图库文件接续

2026-11-03，补 C03/C11 的实际公开商品图片读取：主预览停用旧 Next.js 8000 后，Admin 详情、商品图库及 SKU 选图仍使用恢复库中的旧回环地址。Astro 已有严格的原 30 张商品图地址映射及原始文件，本次复用该映射，让 Vite 将同一批公开图片作为 Admin 构建资产打包。原数据库 URL、图片身份、排序、SKU 关联、缩略图以及保存指令内容保持原样。

`catalog-image-url.ts` 只映射已存在的本地原图。未知路径、外部 CDN、带 query/hash/凭据的旧地址及新 `/assets` 地址保留原值；没有远程文件代理，没有把设备、回收照片或其他私有目录放入公开构建。商品详情原 URL 协议检查保持。Admin 的独立冻结副本验证器同步复制并校验所依赖的公开原图与地址模块，单独 browser 模式无需启动 Astro。

## 验证

在现有主预览只读运行：

```powershell
node retail/admin/node_modules/typescript/bin/tsc --noEmit -p retail/admin/tsconfig.json
cd retail/admin
node tests/local-catalog-read.live.mjs
node node_modules/vite/bin/vite.js build --outDir ../../logs/retail-native/catalog-image-build
```

真实后台登录后，禁止所有非认证后台写请求，依次读取全部 30 张商品图。后台及 Astro 响应字节均与旧 storefront 源文件一致。实际商品详情、商品图片抽屉及 SKU 选图在英文 1440、中文 390 下图片均完成解码，四个运营抽屉仍可读/取消，浏览器异常及业务写请求为 0，无横向溢出。8 个非目标 URL 保持原值。保存相关逻辑未变，本次不重复付款/售后完整流程。

Admin 发布包输出到忽略的自有日志目录，未覆盖正在服务的开发目录。30 张图片的 SHA256 均存在于发布包；Vite 对相同内容去重，实际为 26 个 PNG 文件。第一次按原文件名逐个找输出的验证断言失败，因为去重后的输出使用另一原文件名；随后按全部 30 个内容 SHA256 核验通过，没有将该断言失败冒称通过。

API `/health`、Admin `/account`、Astro 商店及原 iPhone 17e 商品路由均 200；近期服务日志仅发现页面切换取消请求产生的一次 503，随后同一 SKU 图片接口 200，无持续异常。机器证据见 [报告](local-catalog-files-report.json)，截图保存在忽略目录 `logs/retail-native/catalog-read/`。

本项只闭合现有公开商品图的后台读取及发布打包；正式私有文件包、历史文件关系完整对账、C01—C11 全部要求仍未完成。
