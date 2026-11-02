# 本地预览统一更新：审计与演练计划

2026-11-01。目标是让当前9300 Astro使用最新Gin，而不是仅通过隔离测试。最新状态：已切换唯一9100 Gin到项目专属持久99迁移MySQL，实际商店回归通过；共享3306与门禁保持。以下原计划及演练记录保留，实际结果和管理命令见文末及[切换报告](local-preview-cutover-report.json)。

## 已核对

- [只读审计](../../logs/gin-migration/local-preview-upgrade-audit-20261101.json)：MySQL9.6.0，主库eurorefurb_local_trial，276张表、184项外键；86个已应用迁移与当前99文件计划的前缀及SHA一致，未完成步骤0，缺0087—0099共13项。数据和索引估算37,339,136字节，不等于导出大小。当前连接账号可见的其他用户库0，不保证不可见库不存在。
- `foreign_key_checks=1`，`innodb_native_foreign_keys=0`。9.6默认由SQL层处理外键，不能将该状态误写成“外键完全关闭”。专用联合环境使用原生InnoDB模式；切换属于启动选项，需受控重启，不能用session修改代替。参见[MySQL9.6发行说明](https://dev.mysql.com/doc/relnotes/mysql/9.7/en/news-9-6-0.html)及[重启要求](https://dev.mysql.com/doc/refman/26.10/en/replication-features-foreign-keys.html)。
- 当前代码[完整原23组SQL回归](native-sql-99-search-boundaries-report.json)：owner d87bbfe447dd，1807文件冻结、99迁移、37专用数据库、原生FK；23/23通过且日志与二进制SHA核对，所有资源已清理。该23组不包含搜索浏览器，搜索另由[50类HTTP/168组Astro联合](search-option-boundaries-native-report.json)证明。
- 另一个任务“拉取最新代码并启动”确认未计划或执行主升级；当前主Gin为旧86兼容二进制，9300为源码Astro开发服务。新Gin完整schema校验需要99，不能只换binary或绕过校验。

## 下一步演练及更新顺序

1. 从已有显式本地配置读取连接，不输出DSN、密码或业务行。把主库作一致性逻辑备份，包含触发器/事件及精确字符集；文件权限0600且不提交。记录SHA、大小和备份起止时间。确认有足够磁盘空间。
2. 在网络关闭、私有socket、独立目录且原生FK开启的专用MySQL恢复备份。只运行恢复审计/迁移命令，不启动会执行worker/jobs的第二个应用后端。核对每表实际行数及规范化数据指纹、迁移journal及runtime/门禁快照，证明恢复后的86与备份一致。隔离操作不将邮件/支付worker或真实渠道带入。
3. 使用冻结当前99迁移的migrate二进制，对恢复副本运行86→99；核对预期Applied13/Skipped86，再执行一次确认Applied0/Skipped99。检查0097既有重复对预检的影响，失败保留证据并修复迁移策略或来源事实审计，禁止删行去重以凑通过。验证新增表/触发器/约束及未变业务表的行数/指纹；允许0087/0098明确runtime与授权模型版本变化，所有新门禁仍默认关闭。
4. 验证备份恢复和迁移演练后，准备具体本地更新脚本与回滚步骤。主写入需在维护窗口停止，避免备份后变更丢失。受控停止本地Gin及相关worker，记录原PID/二进制/启动参数，保留旧二进制与配置；不停止Medusa/Next。确认MySQL其他使用者与启动方式后，才修改原生FK启动配置并受控重启。禁止kill所有Node或同库启动第二个后端。
5. 在受控主库执行已演练的13项迁移，再启动匹配99的Gin；Astro源码dev继续连接唯一Gin，无需同目录build。保持已有关闭门禁，不能为“可用”批量开启付款/邮件/购物车/价格功能；需要启用的功能另依其验收证据安排。
6. 验证主Gin完整readiness、Medusa9001 health/Admin、原Next8000及9300首页/商店/搜索/详情/账号等受影响路径，查看最近日志。检查30机型、分页/排序/价格/选项及搜索对照。若失败，停止新写入，按已验证备份恢复86和原配置/二进制，不把99库交给旧binary。

## 仍未证明

目前没有完成主库备份/恢复、已有86数据的升级演练或主环境实际切换；不能称“升级就绪”。SQL回归使用专用新库，并不替代主数据恢复演练。整个C01–C11迁移仍未完成。

## 2026-11-01 恢复迁移演练结果与切换修正

[演练报告](local-preview-upgrade-rehearsal-report.json) owner6f185f0e79b6已通过：一致性备份18,721,035字节、0600、275基础表/45,430行、另1视图；恢复后逐表行数/数据SHA、规范后的表DDL及视图/触发器/事件/例程元数据一致。规范仅消除COLLATE已确定字符集时的冗余显式CHARACTER SET，不忽略约束/索引等差异。首次迁移Applied13/Skipped86，第二次Applied0/Skipped99。268张原表数据指纹保持，7张预期runtime/授权/迁移journal表改变，新增18表，新门禁默认关闭。迁移前184、后213个完整外键组合均检查无孤儿。专用服务器、数据和socket已清理，0600备份与报告保留。三次先前核对/演练参数失败的原因及清理也保留，不能把失败当作主迁移执行。

主库演练后仍86，未重启/改配置；7现有URL200，最近Medusa与Gin日志无panic/fatal。应用账号只能看本库，之前“其他可见用户库0”不能用于共享服务范围判断。随后用用户已授权root只读盘点确认**另外45个用户库**，因此不按前述第4步直接改全局配置或重启共享3306。`native-mysql-compatibility.md`已记录默认SQL层模式的TaxSchema非法父项写入复现，不重复把已知未通过模式作为新路径。

调整后的下一步是准备项目专属、持久、私有socket的本机MySQL，开启已验收原生FK、关闭网络/事件调度；在切换窗口停止本项目旧Gin及写worker后重新取一致性备份，恢复/迁移到专属实例，再让唯一的新Gin连接该实例，Astro仍使用原9100 API。共享3306、其他45库与Medusa不变；禁止同库双后端或开启真实支付/邮件。需先准备明确的启动/关闭/重启管理、应用最小权限连接、备份与回滚脚本并验证，再实际切换。专属持久实例及主切换尚未执行，不能称已更新预览或整个C01–C11完成。

## 2026-11-01 实际切换与页面回归

[切换报告](local-preview-cutover-report.json)：先停止已核对旧Gin45145，再取fresh backup owner `9d72f058ad87`并完整重做恢复/13项升级/99项幂等演练。持久实例位于Git忽略的`log/gin-preview-mysql/data`，仅私有socket `/private/tmp/er-gin-preview/mysql.sock`，native FK/checks开启、网络/MySQLX/事件调度关闭。独立随机凭据保存在0700目录中的0600文件，应用账号只有本库DML和SHOW VIEW，无全局或DDL权限。`.env`只原子替换MYSQL_DSN；旧完整配置和旧二进制保留。没有共享3306全局变更、双后端、真实支付/邮件或Medusa重启。

唯一新Gin先由65339接管9100，随后管理命令受控重启到69436且health200。原Astro9300与Ant9200保持地址及样式。实际持久库293表中，291张非迁移日志表与fresh演练完全相同；两张journal仅执行时间不同，99项文件校验和一致且无未完成步骤。268张原业务表与升级前完全一致；逐表实际COUNT与dump解析行数一致，213项完整外键组合无孤儿；共享原86库所有表数据指纹与冻结备份一致。

真实Chrome只读访问42个原Next/Astro页面，21组对应比较全部机型集合、顺序、卡片文字和图片一致，包含中英×390/768/1440及搜索、系列、容量、颜色、价格与相关度排序。无浏览器异常/横向溢出，首卡几何一致；这不等于全站逐像素验收。另直接验证中英商店初始HTML各30卡片/30唯一链接，30详情×两语言均HTTP200；关于/首商品中英×390/1440水合与React Admin匿名登录表单通过，未登录操作业务数据。

Astro配置为`output: 'server'`：构建产物包含服务端渲染程序，商品从当前Gin在请求时渲染。当前HTML已经包含全部30款，不依赖浏览器再补齐；这不是生成30个静态HTML文件的静态站配置。

收尾9001 health/Admin、8000商店、9100 health、9200及9300关于/中英商店均200。近期日志无panic/fatal或意外5xx；`GET /api/store/cart`与`GET /api/store/product-reviews`因保留关闭门禁出现预期503，不能据此称购物、评价已在主预览开放。整个C01–C11仍未完成。

## 本地启停与恢复

从仓库根目录执行；这些命令不构建代码、不修改门禁、不启动支付/邮件worker，也不管理Medusa/Next：

```sh
python3 retail/scripts/preview-api.py status
python3 retail/scripts/preview-api.py start
python3 retail/scripts/preview-api.py restart
python3 retail/scripts/preview-api.py stop
python3 retail/scripts/preview-mysql.py status
```

`start`在已有正确监听者时只检查状态，原PID65339保持的实际验证通过；无监听者时先启动专属MySQL再启动已审计binary，受控restart实际得到69436/health200。遇其他binary、DSN或schema不符时拒绝操作。`stop`仅停止已验证的9100二进制，保留MySQL和数据。确需关闭专属数据库时，先停止Gin，再运行`python3 retail/scripts/preview-mysql.py stop`；后者的init/stop/start/status已实际验证保留数据。重启电脑后可运行API start；尚未安装操作系统自启动，Redis须按原本地配置可用。

首次切换专用的`switch-local-preview.py`已成功执行，拒绝重复运行。`preview-mysql.py init/restore`同样不覆盖已有实例/数据库，不用于日常启动。

只读数据验收可执行`python3 retail/scripts/verify-local-preview.py`：核对本轮冻结数据、99校验和、行数、213外键和保留原86库。它是切换基线验收，不是允许业务写入后的日常健康检查；之后产生新业务事实时指纹变化应保留并对账。凭据和业务dump仅在忽略目录，勿展示或提交。

恢复材料是`log/gin-preview-mysql/original-backend.env`、旧`log/gin-local-api-20261031`、共享3306保留的86库，以及`logs/gin-migration/preview-upgrade-9d72f058ad87/preview-86-private.sql`。自动失败回退已在切换脚本中实现，但成功切换后没有执行反向回退演练。需要反向切换时，应先受控停止唯一新Gin和相关写worker，保留99库新备份并核对切换后的新增事实；只有确认没有待保留写入，才恢复原.env并启动匹配86的旧二进制。不得把旧二进制连到99库，或覆盖/丢弃新增业务事实。现有共享86库不需要为回退而删库重导；持久99目录和证据保持。

2026-11-01 后续无schema二进制更新：DHL演示由owner42fb2f0d44d4完整23组SQL冻结源码更新，主PID372，运行SHA更新到cutover事实，旧binary/facts保留在binary-refresh-42fb2f0d44d4。更新前后业务/外键/迁移校验和一致；不改变原86→99数据升级的历史证明。见[详细结果与运行边界](astro-shipping-demo.md)和[联合报告](shipping-demo-native-report.json)。

## 2026-11-01 主101→102与恢复边界

[实际102报告](local-preview-102-report.json)：`rehearse-local-preview-102.py`先完成独立101备份恢复，再运行0102、幂等重放、精确逆向101及再次102；`upgrade-local-preview-102.py`在停止唯一Gin后取得新鲜备份并完整复演，以新鲜演练数据/DDL为主升级基线。两项均已执行成功；不要重复执行升级脚本。最新fresh owner为15bf79e93807，0600备份位于忽略目录`logs/gin-migration/preview-102-upgrade-15bf79e93807/preview-101-private.sql`。

`cart-102-recovery.py`只供受控失败恢复：数据库必须为指定私有库，原业务数据和101 journal/校验和不变，新开关只能未激活且收据为空；激活状态、收据或额外事实均在首个修改前拒绝。不关闭外键、不覆盖重导业务表，也不授予权限。实际克隆逆向验证214FK零孤儿，重新102为215FK零孤儿；主成功路径没有执行逆向。保留旧binary和facts在`log/gin-preview-mysql/schema-upgrade-102/`。新业务事实出现后不能直接使用该窄逆向或覆盖备份。

现有`preview-api.py start/stop/status/restart`及只读`verify-local-preview.py`已支持102。主299表/297非journal数据与新鲜演练一致，268原表及保留共享86指纹一致。随后主无车偏好验收发现缺本地STOREFRONT_ORIGIN；仅增加9300 origin并受控唯一Gin重启到42442，配置原文备份与摘要在忽略目录`log/gin-preview-mysql/origin-fix-102/`。二进制仍为上述已验收102版本，数据库门禁与业务数据不变。

9300是既有Astro dev PID29403，源码HMR，本次没有重启或构建；此前“旧Astro构建未更新”的部分叙述有误，以实际进程和新报告为准。中英商店初始HTML各30机型；主四组无车偏好、42商店页/21组来源对照及58页/16组SEO回归通过，来源Next首轮中文help双JSON-LD异常记录保留。现有Medusa/Admin、Next、Gin、React Admin及Astro入口均200，完整C01–C11仍未完成。
