# 仓库员工带照片入库

本批复用既有Gin设备入库事务和Ant批次表单，将其接到“仓库设备→设备入库”。员工先在授权仓库及销售/退回分类中选择SKU，再登记最多200台设备及每台最多8张照片。保留原SN/无线标识、SKU成色/电池、自有来源、sale→available、returned→inspection等规则；不增加数量库存。

专用路径以 `/api/admin/warehouse/locations/:location_id/device-intake/:category` 开头：

- GET `/variants`：共享商品定义的20条分页搜索，支持商品名称、配置标题、SKU及原有型号缩写/容量/成色/电池/中文颜色组合；只投影ID、商品ID/名称、配置标题、SKU。没有价格、成本、metadata、库存level或客户关系。SKU能否入库仍由实际配置读取/写入检查，选择器不改变原商品定义的共享范围。
- GET/POST `/products/:id/variants/:variant_id`：读取配置/当前来源/近期设备，或提交整批登记。
- POST `/photos`：单图上传、真实文件头/长度/SHA256及不可变操作ID，沿用原公开设备图语义。

专用scope由路由参数建立。所有入口在事务内校验当前warehouse主体、有效账号、明确分类读/写权限、有效仓库及selected/all范围。读取要求warehouse.intake.read或warehouse.return.read；写入/上传还需对应write。回收类别、supplier和全店employee主体拒绝，不能借此开放全店商品或原入库端点。

来源只列当前仓库有效self来源；近期50台只列当前SKU、仓库和类别。批次的每台类别必须与路径相同，来源必须仍绑定该仓库且为self；提交前和重放前都查真实来源。前端锁定类别仅为操作提示，篡改body不能绕过服务器限制。来源/类别改变、范围撤销、仓库删除后旧成功请求也必须重新授权。

0061给入库收据及设备照片增加context_key。空字符串保留原全店请求；仓库请求绑定类别与仓库ID。读取摘要也包含scope和过滤后的来源，同操作不能跨全店/仓库或跨类别恢复为另一份请求。旧收据不自动变成仓库收据。照片已上传但入库失败时保留已存文件，允许修改批次重试；未关联图片清理仍未提供。

共用表单沿用Ant Drawer/Form.List/Select/InputNumber/Upload、相机与JPEG缩放；上传不确定时阻止提交并可原请求重试。商品选择器不使用全店商品接口。保存后回到仓库列表即可看到设备及照片，刷新保留；只读员工无入库按钮。全店入库原路径与行为保留。

验证以暂存源码独立快照执行，排除未提交Mollie通知。60项迁移及AdminAuthentication/InventoryStockEdits/ProductInventory隔离回归覆盖：来源/近期设备范围、无敏感关联的SKU搜索及型号缩写、容量、等级、电池、中文颜色的组合筛选、全店接口拒绝、跨仓/供应商/类别篡改、回收拒绝、分类照片上传、收据context绑定、4并发仅一次创建、撤权后旧请求拒绝、来源迁仓后拒绝重放、末尾收据回滚、销售/退回状态；父用例对数量库存完整指纹验证不变。

真实浏览器联合命令 `python retail/scripts/test-warehouse-inventory-joint.py --warehouse-intake`：管理员在UI新建独立仓库员工和分类权限，员工登录后选SKU、上传照片、登记销售设备，再登记退回设备；关闭抽屉/刷新后列表及照片可见。直接跨仓/全店请求拒绝，中英/390宽度检查。MySQL回查两份带scope的入库收据、一份照片、两次入库事件、设备位置与状态及精确数量库存不变，归属容器/网络清理后写报告。

合成API浏览器另验共用表单在专用路径下的照片/整批丢响应同请求恢复、分类锁定、8图移除、冲突重读、空来源和只读隐藏。Go全包、请求大小路由边界、Admin类型/构建和原19项浏览器回归一并检查。

证据：warehouse-intake-report.json、warehouse-intake-joint-report.json。仍需设备编辑、检测、转退回、跨SKU/移动入库、完整配货发货与前台履约，整个前台闭环保持未验收。
