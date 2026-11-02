# Windows 商品录入主流程

2026-11-02，本机已有商品/SKU/价格/初始仓库库存实现，但主库product.create、inventory.create_level、catalog_variant_create_runtime和catalog_price_edit_runtime仍关闭，无法完成新商品录入。此次只启用已有命令，不改Ant界面或生产默认配置。

`start.bat`现在支持显式LOCAL_PREVIEW_PRODUCT_ENTRY_ENABLED=true，调用configure-local-preview --product-entry。命令仍严格限定development、TCP 127.0.0.1:13307和eurorefurb_local_trial；同一事务检查两项capability version1及两个runtime version1/正revision，才开启四个开关。不重置revision或ERP编号，不创建商品、价格、库存或预留。production/错误数据库实际命令均拒绝。本机忽略.env已设置；当前服务无需重启即可读取这些数据库开关。

固定子流程：`python retail/scripts/test-local-customer-account.py --scenario=product-entry`。复用独立恢复库、专属Redis前缀、原生19100/19200/19300和可写Astro源码副本，不启动额外主库worker，不调用支付或外部邮件。

成功owner b5a0df471e79：实际Ant登录→新建已发布商品及128/256 GB初始选项→新建128 GB SKU并分配ERP11565和独立库存项→录入EUR基础价129.123456789012345678→同商品刷新/公开API读回→European Warehouse录入现货3、在途0。实际Astro /dk-en和/dk-zh商品手机页显示同标题、可购买，无横向溢出。页面按原货币格式显示129.12，API保留18位小数。新增商品无图片，沿用原“图片即将上线”占位；本轮没有上传图片或加购付款。

SQL确认商品/SKU/价格/库存层收据各1；主八张商品/价格/库存/收据表完整数据指纹不变，源十二账户/交易入口表仍0。4条专属Redis键、数据库及授权和三个进程已清理。早期测试字段标签、嵌套Drawer定位和商品收据表名错误已修正，失败报告保留；没有为测试改业务实现。最终运行直接通过，不将早期失败改写成成功。

主预览 /products 实际中英1440/390宽只读检查通过：creation API200/private-no-store/开关true，打开原新建Drawer并取消，商品写请求0、浏览器错误0。主四项收据仍0，原European库存50/预留1。Go配置命令/catalog/inventory测试、配置命令vet、Python/JS/PowerShell语法及diff检查通过。

该证据只补商品录入主链路。图片、已有选项编辑/共享定义、地区/数量价格、SKU关系等各自开关与此前迁移范围独立；真实当前数据组合及C03/C05/C11其余条目继续开放。没有把此子流程称作完整购物或整期验收。
