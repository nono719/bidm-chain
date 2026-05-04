## 4.4 数据库设计

### 4.4.1 E-R 模型

系统包含 `User`、`Domain`、`Device`、`CrossDomainAuthSession`、`DomainTrustPolicy`、`OracleNode`、`SystemSetting`、`DeviceStateUpdate`、`ProtectedOperation`、`ChainAnchor`、`AuditLog` 共 11 个实体。

实体关系说明如下：

- 1 个域（`Domain`）可包含 n 个用户（`User`）与 n 个设备（`Device`）。
- 1 个设备（`Device`）可产生 n 条跨域认证会话（`CrossDomainAuthSession`）与 n 条状态更新（`DeviceStateUpdate`）。
- 1 个设备（`Device`）可产生 n 条受保护操作记录（`ProtectedOperation`）与 n 条审计记录（`AuditLog`）。
- 1 条跨域信任策略（`DomainTrustPolicy`）由源域与目标域共同定义，域之间是 n:n 的策略映射关系。
- 审计记录（`AuditLog`）与链上锚定记录（`ChainAnchor`）用于记录关键业务操作和链上回执。

E-R 图见：

- `docs/db_design/er_model.mmd`（Mermaid 源文件）
- `docs/db_design/er_model.svg`
- `docs/db_design/er_model.png`

### 4.4.2 数据表

本系统数据表共 11 张。物理表名及说明如表 4.1 所示。

#### 表 4.1 表清单

| 序号 | 中文名称 | 物理表名（GORM） | 说明 |
|---|---|---|---|
| 1 | 用户表 | `users` | 存储系统用户、角色与所属域信息 |
| 2 | 管理域表 | `domains` | 存储多管理域基础信息 |
| 3 | 设备身份表 | `devices` | 存储设备 DID、凭据与运行状态 |
| 4 | 跨域认证会话表 | `cross_domain_auth_sessions` | 存储跨域认证申请、审批、撤销与令牌信息 |
| 5 | 域间信任策略表 | `domain_trust_policies` | 存储域到域授权策略与权限范围 |
| 6 | 预言机节点表 | `oracle_nodes` | 存储预言机节点公钥、状态与心跳时间 |
| 7 | 系统配置表 | `system_settings` | 存储系统级参数配置（如门限值） |
| 8 | 设备状态更新表 | `device_state_updates` | 存储预言机上报的设备状态结果 |
| 9 | 受保护操作记录表 | `protected_operations` | 存储跨域认证后受保护操作执行记录 |
| 10 | 链上锚定表 | `chain_anchors` | 存储业务摘要上链后的交易回执信息 |
| 11 | 审计日志表 | `audit_logs` | 存储全链路审计事件与细节 JSON |

---

#### 表 4.2 用户表（`users`）

主键：`id`  
业务主键：`username`  
索引：`username`（唯一索引）、`domain_code`（普通索引）

| 序号 | 中文名称 | 列名 | 数据类型 | 非空 | 约束/索引 | 说明 |
|---|---|---|---|---|---|---|
| 1 | 用户ID | `id` | BIGINT UNSIGNED | Y | PK | 自增主键 |
| 2 | 用户名 | `username` | VARCHAR(64) | Y | UK | 登录名，全局唯一 |
| 3 | 密码哈希 | `password_hash` | VARCHAR(255) | Y |  | BCrypt 哈希 |
| 4 | 显示名 | `display_name` | VARCHAR(128) | Y | 默认空串 | 页面展示名称 |
| 5 | 角色 | `role` | VARCHAR(32) | Y |  | 如 `ADMIN`、`DOMAIN_ADMIN` |
| 6 | 所属域编码 | `domain_code` | VARCHAR(64) | Y | IDX | 关联 `domains.code`（逻辑外键） |
| 7 | 创建时间 | `created_at` | DATETIME | Y |  | 记录创建时间 |

#### 表 4.3 管理域表（`domains`）

主键：`id`  
业务主键：`code`  
索引：`code`（唯一索引）

| 序号 | 中文名称 | 列名 | 数据类型 | 非空 | 约束/索引 | 说明 |
|---|---|---|---|---|---|---|
| 1 | 域ID | `id` | BIGINT UNSIGNED | Y | PK | 自增主键 |
| 2 | 域编码 | `code` | VARCHAR(64) | Y | UK | 管理域唯一编码 |
| 3 | 域名称 | `name` | VARCHAR(128) | Y |  | 管理域中文名称 |
| 4 | 创建时间 | `created_at` | DATETIME | Y |  | 记录创建时间 |

#### 表 4.4 设备身份表（`devices`）

主键：`id`  
业务主键：`device_did`  
索引：`device_did`（唯一索引）、`domain_code`（普通索引）、`lifecycle`（普通索引）

| 序号 | 中文名称 | 列名 | 数据类型 | 非空 | 约束/索引 | 说明 |
|---|---|---|---|---|---|---|
| 1 | 设备ID | `id` | BIGINT UNSIGNED | Y | PK | 自增主键 |
| 2 | 设备DID | `device_did` | VARCHAR(128) | Y | UK | 设备去中心化标识 |
| 3 | 所属域编码 | `domain_code` | VARCHAR(64) | Y | IDX | 关联 `domains.code`（逻辑外键） |
| 4 | 设备显示名 | `display_name` | VARCHAR(128) | Y |  | 设备名称 |
| 5 | 凭据 | `credential` | VARCHAR(255) | Y |  | 设备凭据摘要 |
| 6 | 设备类型 | `device_type` | VARCHAR(64) | Y | 默认 `UNKNOWN` | 类型分类 |
| 7 | 公钥JWK | `public_key_jwk` | TEXT | N |  | 设备公钥信息 |
| 8 | 元数据JSON | `metadata_json` | TEXT | N |  | 设备扩展属性 |
| 9 | 生命周期 | `lifecycle` | VARCHAR(32) | Y | IDX，默认 `ACTIVE` | 设备生命周期状态 |
| 10 | 运行状态 | `runtime_state` | VARCHAR(64) | Y | 默认 `UNKNOWN` | 最近运行态 |
| 11 | 创建时间 | `created_at` | DATETIME | Y |  | 创建时间 |
| 12 | 更新时间 | `updated_at` | DATETIME | Y |  | 更新时间 |

#### 表 4.5 跨域认证会话表（`cross_domain_auth_sessions`）

主键：`id`  
业务主键：`request_id`  
索引：`request_id`（唯一索引）、`device_did`（普通索引）、`status`（普通索引）

| 序号 | 中文名称 | 列名 | 数据类型 | 非空 | 约束/索引 | 说明 |
|---|---|---|---|---|---|---|
| 1 | 会话ID | `id` | BIGINT UNSIGNED | Y | PK | 自增主键 |
| 2 | 请求ID | `request_id` | VARCHAR(64) | Y | UK | 跨域认证唯一请求号 |
| 3 | 设备DID | `device_did` | VARCHAR(128) | Y | IDX | 关联 `devices.device_did`（逻辑外键） |
| 4 | 源域编码 | `from_domain_code` | VARCHAR(64) | Y |  | 认证发起域 |
| 5 | 目标域编码 | `to_domain_code` | VARCHAR(64) | Y |  | 认证目标域 |
| 6 | 资源标识 | `resource` | VARCHAR(255) | Y | 默认空串 | 受保护资源 |
| 7 | 权限级别 | `permission` | VARCHAR(64) | Y | 默认空串 | 权限请求级别 |
| 8 | 会话时长秒 | `ttl_seconds` | INT | Y | 默认 1800 | 会话有效秒数 |
| 9 | 挑战码 | `challenge` | VARCHAR(128) | Y |  | 挑战应答值 |
| 10 | 状态 | `status` | VARCHAR(32) | Y | IDX | 例如 `PENDING/VERIFIED/REVOKED` |
| 11 | 申请人 | `requested_by` | VARCHAR(64) | Y | 默认空串 | 发起用户名 |
| 12 | 验签时间 | `signature_verified_at` | DATETIME | N |  | 验签通过时间 |
| 13 | 通过验证时间 | `verified_at` | DATETIME | N |  | 会话验证通过时间 |
| 14 | 过期时间 | `expires_at` | DATETIME | N |  | 会话失效时间 |
| 15 | 审批人 | `approved_by` | VARCHAR(64) | Y | 默认空串 | 审批用户名 |
| 16 | 审批时间 | `approved_at` | DATETIME | N |  | 审批完成时间 |
| 17 | 撤销人 | `revoked_by` | VARCHAR(64) | Y | 默认空串 | 撤销操作人 |
| 18 | 撤销时间 | `revoked_at` | DATETIME | N |  | 撤销时间 |
| 19 | 撤销原因 | `revoke_reason` | VARCHAR(255) | Y | 默认空串 | 撤销原因说明 |
| 20 | 令牌JSON | `token_json` | TEXT | N |  | 会话令牌扩展内容 |
| 21 | 创建时间 | `created_at` | DATETIME | Y |  | 创建时间 |

#### 表 4.6 域间信任策略表（`domain_trust_policies`）

主键：`id`  
业务主键：无（由 `from_domain_code + to_domain_code` 形成业务组合语义）  
索引：`from_domain_code`（普通索引）、`to_domain_code`（普通索引）

| 序号 | 中文名称 | 列名 | 数据类型 | 非空 | 约束/索引 | 说明 |
|---|---|---|---|---|---|---|
| 1 | 策略ID | `id` | BIGINT UNSIGNED | Y | PK | 自增主键 |
| 2 | 源域编码 | `from_domain_code` | VARCHAR(64) | Y | IDX | 源管理域 |
| 3 | 目标域编码 | `to_domain_code` | VARCHAR(64) | Y | IDX | 目标管理域 |
| 4 | 策略级别 | `policy_level` | VARCHAR(32) | Y | 默认 `ALLOW` | 允许/拒绝策略 |
| 5 | 允许权限集 | `allowed_perms` | VARCHAR(255) | Y | 默认 `READ` | 逗号分隔权限集合 |
| 6 | 允许资源集 | `allowed_res` | VARCHAR(255) | Y | 默认 `*` | 资源白名单 |
| 7 | 是否启用 | `enabled` | BOOLEAN | Y | 默认 `true` | 策略状态 |
| 8 | 更新人 | `updated_by` | VARCHAR(64) | Y | 默认空串 | 最后更新人 |
| 9 | 创建时间 | `created_at` | DATETIME | Y |  | 创建时间 |
| 10 | 更新时间 | `updated_at` | DATETIME | Y |  | 更新时间 |

#### 表 4.7 预言机节点表（`oracle_nodes`）

主键：`id`  
业务主键：`node_name`  
索引：`node_name`（唯一索引）

| 序号 | 中文名称 | 列名 | 数据类型 | 非空 | 约束/索引 | 说明 |
|---|---|---|---|---|---|---|
| 1 | 节点ID | `id` | BIGINT UNSIGNED | Y | PK | 自增主键 |
| 2 | 节点名称 | `node_name` | VARCHAR(64) | Y | UK | 预言机节点唯一名 |
| 3 | 节点公钥 | `public_key` | VARCHAR(255) | Y |  | 节点公钥（PEM） |
| 4 | 节点状态 | `status` | VARCHAR(32) | Y | 默认 `ACTIVE` | 节点可用状态 |
| 5 | 最后在线时间 | `last_seen` | DATETIME | Y |  | 最近心跳时间 |
| 6 | 创建时间 | `created_at` | DATETIME | Y |  | 创建时间 |
| 7 | 更新时间 | `updated_at` | DATETIME | Y |  | 更新时间 |

#### 表 4.8 系统配置表（`system_settings`）

主键：`id`  
业务主键：`conf_key`  
索引：`conf_key`（唯一索引）

| 序号 | 中文名称 | 列名 | 数据类型 | 非空 | 约束/索引 | 说明 |
|---|---|---|---|---|---|---|
| 1 | 配置ID | `id` | BIGINT UNSIGNED | Y | PK | 自增主键 |
| 2 | 配置键 | `conf_key` | VARCHAR(64) | Y | UK | 配置项键名 |
| 3 | 配置值 | `conf_value` | VARCHAR(255) | Y | 默认空串 | 配置项值 |
| 4 | 更新人 | `updated_by` | VARCHAR(64) | Y | 默认空串 | 最后更新人 |
| 5 | 更新时间 | `updated_at` | DATETIME | Y |  | 更新时间 |
| 6 | 创建时间 | `created_at` | DATETIME | Y |  | 创建时间 |

#### 表 4.9 设备状态更新表（`device_state_updates`）

主键：`id`  
业务主键：无  
索引：`device_did`（普通索引）、`state_label`（普通索引）、`severity`（普通索引）

| 序号 | 中文名称 | 列名 | 数据类型 | 非空 | 约束/索引 | 说明 |
|---|---|---|---|---|---|---|
| 1 | 状态记录ID | `id` | BIGINT UNSIGNED | Y | PK | 自增主键 |
| 2 | 设备DID | `device_did` | VARCHAR(128) | Y | IDX | 关联 `devices.device_did`（逻辑外键） |
| 3 | 在线状态 | `online` | BOOLEAN | Y |  | 在线/离线 |
| 4 | 固件校验通过 | `firmware_valid` | BOOLEAN | Y |  | 固件合法性 |
| 5 | 证书校验通过 | `cert_valid` | BOOLEAN | Y |  | 证书合法性 |
| 6 | 风险评分 | `score` | INT | Y |  | 设备综合评分 |
| 7 | 状态标签 | `state_label` | VARCHAR(64) | Y | IDX | 业务状态标签 |
| 8 | 严重等级 | `severity` | INT | Y | IDX | 告警级别 |
| 9 | 描述信息 | `message` | VARCHAR(255) | Y |  | 状态说明 |
| 10 | 交易哈希 | `tx_hash` | VARCHAR(128) | N |  | 对应锚定交易哈希 |
| 11 | 区块高度 | `block_height` | BIGINT UNSIGNED | Y |  | 对应区块高度 |
| 12 | 创建时间 | `created_at` | DATETIME | Y |  | 创建时间 |

#### 表 4.10 受保护操作记录表（`protected_operations`）

主键：`id`  
业务主键：无  
索引：`device_did`（普通索引）

| 序号 | 中文名称 | 列名 | 数据类型 | 非空 | 约束/索引 | 说明 |
|---|---|---|---|---|---|---|
| 1 | 操作记录ID | `id` | BIGINT UNSIGNED | Y | PK | 自增主键 |
| 2 | 设备DID | `device_did` | VARCHAR(128) | Y | IDX | 关联设备标识 |
| 3 | 操作域编码 | `domain_code` | VARCHAR(64) | Y |  | 操作发起域 |
| 4 | 操作类型 | `operation` | VARCHAR(64) | Y |  | 敏感操作名 |
| 5 | 操作载荷 | `payload` | TEXT | N |  | 请求体快照 |
| 6 | 创建人 | `created_by` | VARCHAR(64) | Y |  | 操作执行人 |
| 7 | 创建时间 | `created_at` | DATETIME | Y |  | 创建时间 |

#### 表 4.11 链上锚定表（`chain_anchors`）

主键：`id`  
业务主键：无（`biz_type + biz_ref + tx_hash` 为业务联合识别）  
索引：`biz_ref`（普通索引）

| 序号 | 中文名称 | 列名 | 数据类型 | 非空 | 约束/索引 | 说明 |
|---|---|---|---|---|---|---|
| 1 | 锚定ID | `id` | BIGINT UNSIGNED | Y | PK | 自增主键 |
| 2 | 业务类型 | `biz_type` | VARCHAR(64) | Y |  | 如 `oracle_report` |
| 3 | 业务引用 | `biz_ref` | VARCHAR(128) | Y | IDX | 业务对象标识 |
| 4 | 摘要值 | `digest` | VARCHAR(128) | Y |  | 上链前摘要 |
| 5 | 交易哈希 | `tx_hash` | VARCHAR(128) | Y |  | Fabric 交易哈希 |
| 6 | 区块高度 | `block_height` | BIGINT UNSIGNED | Y |  | 链上区块高度 |
| 7 | 创建时间 | `created_at` | DATETIME | Y |  | 创建时间 |

#### 表 4.12 审计日志表（`audit_logs`）

主键：`id`  
业务主键：无  
索引：`module`（普通索引）、`subject_did`（普通索引）、`tx_hash`（普通索引）

| 序号 | 中文名称 | 列名 | 数据类型 | 非空 | 约束/索引 | 说明 |
|---|---|---|---|---|---|---|
| 1 | 审计ID | `id` | BIGINT UNSIGNED | Y | PK | 自增主键 |
| 2 | 模块名 | `module` | VARCHAR(64) | Y | IDX | 模块分类 |
| 3 | 动作名 | `action` | VARCHAR(64) | Y |  | 操作动作 |
| 4 | 操作人 | `operator` | VARCHAR(64) | Y |  | 执行用户名 |
| 5 | 结果 | `result` | VARCHAR(32) | Y |  | `OK/ERR` 等 |
| 6 | 合约名 | `contract` | VARCHAR(64) | Y | 默认空串 | 链上合约名 |
| 7 | 方法名 | `method` | VARCHAR(64) | Y | 默认空串 | 合约方法名 |
| 8 | 来源 | `from` | VARCHAR(96) | Y | 默认空串 | 来源系统/地址 |
| 9 | 主体DID | `subject_did` | VARCHAR(128) | Y | IDX | 关联设备主体 |
| 10 | 交易哈希 | `tx_hash` | VARCHAR(128) | N | IDX | 链上交易哈希 |
| 11 | 区块高度 | `block_height` | BIGINT UNSIGNED | Y |  | 对应区块高度 |
| 12 | Gas消耗 | `gas_used` | BIGINT UNSIGNED | Y |  | 资源消耗统计 |
| 13 | 消息 | `message` | VARCHAR(255) | N |  | 描述信息 |
| 14 | 详情JSON | `detail_json` | TEXT | N |  | 扩展审计内容 |
| 15 | 发生时间 | `occurred_at` | DATETIME | Y | autoCreateTime | 事件发生时间 |

### 4.4.3 存储过程、函数与触发器信息

按当前项目源码检索结果：

- 未定义数据库存储过程（`CREATE PROCEDURE`）。
- 未定义数据库函数（`CREATE FUNCTION`）。
- 未定义触发器（`CREATE TRIGGER`）。

当前数据库构建方式为 `GORM AutoMigrate` 自动建表与索引，业务逻辑在应用层（Go 后端）实现。
