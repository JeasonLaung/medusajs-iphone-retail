# 受保护业务值的目标信封

日期：2026-10-27。`retail/backend/internal/protected`实现目标通用加密组件；已验证人工MySQL二进制持久化及轮换回滚。未配置生产密钥、导入真实配置或启动外部provider；受保护唯一键比较已有专用组件，尚未接入HTTP读取授权和导入器，因此不能据此验收T006/T016/T018。

## 固定格式与上下文

二进制格式为`ERS`三字节、版本字节1、key ID长度一字节、key ID的ASCII字节、12字节随机nonce、密文及16字节tag。key ID为1—64字符字母/数字开头的字母数字或`_.-`，无自动读取外部密钥行为；所有header字节纳入认证。版本、长度和最大体积严格校验，未知/缺失key返回固定错误，绝不尝试明文、旧格式或COOKIE_SECRET回退。

AES-256-GCM使用Go标准库`cipher.NewGCMWithRandomNonce`生成nonce，调用者不能自行指定。单密钥必须在2^32次加密前轮换；运行计数、密钥保管/备份、恢复演练和轮换调度仍待技术底座。构造器只接受32字节密钥材料，拒绝全零、重复材料不同ID、缺活动key或无效ID；不能把密码直接当key。构造后密钥环不可变，修改调用者的配置切片不会改动已构造实例。

AAD由域标识`eurorefurb.business-secret.v1`、完整header、用途、目标表、目标列、精确业务行ID六个长度前缀项组成。用途/表/列与key ID使用同一受限标识符格式；行ID保留有效UTF-8原文、不trim或lower、1—191码点。该编码避免字符串拼接歧义，并阻止相同密钥下跨用途、表、列或行替换。权限检查仍由调用者负责；持有密钥并不等于有业务读取授权。

明文是原始字节，不强制文本或JSON，不改变大小写、空格或非UTF-8数据；上限1MiB，超出拒绝，不截断。空字节值也生成非空认证密文；SQL NULL由外层字段可NULL规则处理，不能将NULL或空密文当作空秘密。Go堆和标准库内部密钥状态没有物理内存擦除保证，不能宣称此API销毁所有秘密副本。

## 轮换与错误

新密文使用活动key ID；读取按密文header选择仍保留的旧key。旧ID未知时明确不可读，不使用当前key猜测。轮换先鉴权、在完整旧上下文解密，再按相同业务上下文用新活动key密封，数据库版本检查和更新必须处于调用者事务；失败整体回滚。未完成迁移前不能删除仍有引用的旧key。更换行ID/字段本身需显式重密封，不能复制原信封。

ErrConfig、ErrEnvelope、ErrKeyUnavailable均为固定消息，不带原值、key、行ID、密文或底层解密异常。调用者不能将明文、密钥、原请求/SQL或解密错误上下文写日志；当前旧Gin草稿的通用日志/SQL配置仍须按T010审查，不能只接入组件便假定不会泄漏。

## 已执行证据

`go test ./internal/protected`覆盖6组测试：空/非ASCII/二进制/1MiB边界往返、每字节篡改/每长度截断/尾部追加、全部上下文维度错配及编码歧义、旧新key读取/重密封/缺key/错材料、构造输入错误，以及64路相同明文加密与读取、样本nonce不重复。并发测试是功能验证；本机CGO未启用，未运行race检测器。最大使用量限制不是样本nonce检查能够证明的属性。

`TestRegistryMySQL/protected_envelope_binary_sql_roundtrip_and_rotation`在同一隔离MySQL8.4.11人工库中执行LONGBLOB写入/读取；含中文、零字节、0xff和尾空格原值无损，错误行绑定拒绝；轮换更新回滚后旧密文逐字节不变，正式更新后仅新key仍可读取；SQL NULL保持NULL且不能直接解密。数据库fixture结束核对所有权清理。该子测试与现有登记/源适配子测试一起记录在`mysql-type-fixture-report.json`，附测试二进制hash，不含业务行数据。

复现使用[登记组件的隔离测试流程](mysql-unique-key-registry.md)，重新编译Linux registry.test后传入`test-mysql-types.py --registry-test-binary`。普通go test跳过专用MySQL项，报告72项为71项SQL检查加1套包含多个Go子测试的集成套件，不能把72解释为72个完整业务场景。

## 下一步

[受保护唯一索引](protected-unique-index.md)已实现独立HMAC定位、加密完整值比较、索引配置认证及联系人双约束同事务人工验证。普通联系人路径仍拒绝，专用SecretRegistry路径需要显式密钥和业务密文；真实业务/导入入口尚未接通，28条源长文本唯一键hold数量不变。全部真实源格式解密/新信封持久化、备份与恢复及外部任务禁止重放仍须完成。
