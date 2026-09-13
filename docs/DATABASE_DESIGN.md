# 烽火台（Fenghuo） 数据库设计（V1）

> 数据库：**PostgreSQL 18**（最低兼容 16）。PG 建库即 UTF-8，无字符集/排序规则声明负担
> 所有表统一包含：`id`（`BIGINT GENERATED ALWAYS AS IDENTITY` 主键）、`created_at`、`updated_at`（`TIMESTAMPTZ`，微秒精度），下文不再重复列出
> 布尔语义统一使用原生 `BOOLEAN`，不用 TINYINT 模拟；枚举语义词（role/type/status/disposition）统一 `VARCHAR`，不用 PG ENUM（见 §11）
> 删除策略：**全部硬删除（真实 DELETE）**。管理类表删除前由应用层校验引用关系：模板被渠道引用、渠道被路由规则引用、接入源下存在规则或告警时均拒绝删除；告警与投递记录按保留策略（`FENGHUO_ALERT_RETENTION_DAYS`，默认 30 天）定时物理清理

---

## 1. users — 用户

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | BIGINT | PK, GENERATED ALWAYS AS IDENTITY | |
| username | VARCHAR(64) | NOT NULL, UNIQUE | 登录名（OAuth 自动创建时可用飞书昵称+后缀保证唯一） |
| password_hash | VARCHAR(255) | NULL | bcrypt；纯 OAuth 用户可为空（禁止密码登录） |
| name | VARCHAR(64) | NOT NULL DEFAULT '' | 展示名（飞书同步） |
| email | VARCHAR(128) | NOT NULL DEFAULT '' | 邮箱（飞书同步，可能为空） |
| avatar_url | VARCHAR(512) | NOT NULL DEFAULT '' | 头像（飞书同步） |
| role | VARCHAR(16) | NOT NULL DEFAULT 'viewer' | `admin` / `editor` / `viewer`（能力矩阵见 REQUIREMENTS FR-5.4） |
| enabled | BOOLEAN | NOT NULL DEFAULT TRUE | 禁用后 JWT 即时失效 |
| is_bootstrap | BOOLEAN | NOT NULL DEFAULT FALSE | 引导创建的初始管理员（FR-5.1，setup 接口创建）；受保护不可降级/禁用/删除；部分唯一索引 `uq_users_single_bootstrap` 保证全表至多一行 TRUE |
| last_login_at | TIMESTAMPTZ | NULL | 最近登录时间 |
| password_changed_at | TIMESTAMPTZ | NULL | 密码最近修改时间（迁移 0011）；JWTAuth 比较 token 签发时间与该字段，让改密前签发的 access token 立即失效；纯 OAuth 用户为 NULL |

设计说明：
- `role` 用 VARCHAR 而非 PG ENUM：`ALTER TYPE ... ADD VALUE` 不能在事务里随便跑，VARCHAR 加角色零 DDL
- `password_hash` 可空 → 纯 OAuth 用户没有本地密码，登录时按 NULL 判断走 OAuth 路径
- 原 MySQL 草案的 `status TINYINT(1/0)` 统一改为原生 `enabled BOOLEAN`，与全库布尔字段风格一致

## 2. oauth_identities — OAuth 身份绑定

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | BIGINT | PK | |
| user_id | BIGINT | NOT NULL, INDEX | 关联 users.id |
| provider | VARCHAR(32) | NOT NULL | V1 恒为 `feishu`，预留扩展 |
| provider_user_id | VARCHAR(128) | NOT NULL | 飞书 open_id |
| provider_union_id | VARCHAR(128) | NOT NULL DEFAULT '' | 飞书 union_id（跨应用唯一，备用） |
| extra | JSONB | NULL | 原始用户信息快照（昵称/头像/租户等），排查用 |

**UNIQUE KEY `(provider, provider_user_id)`** — 同一飞书账号只能绑一个本地用户

## 3. refresh_tokens — 刷新令牌

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | BIGINT | PK | |
| user_id | BIGINT | NOT NULL, INDEX | |
| token_hash | CHAR(64) | NOT NULL, UNIQUE | SHA-256(token) 的 hex，不存明文 |
| expires_at | TIMESTAMPTZ | NOT NULL | 过期时间 |
| revoked | BOOLEAN | NOT NULL DEFAULT FALSE | 是否已吊销（登出/改密码时置 TRUE） |
| replaced_by | CHAR(64) | NOT NULL DEFAULT '' | 轮换后新 token 的 hash，用于检测 refresh token 重用（重放即全部吊销） |
| client_ip | VARCHAR(45) | NOT NULL DEFAULT '' | 签发时 IP（兼容 IPv6） |
| user_agent | VARCHAR(255) | NOT NULL DEFAULT '' | |

说明：refresh token 轮换 + 重用检测，是 JWT 双 token 方案的标准做法

## 4. sources — 告警接入源（Alertmanager 实例）

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | BIGINT | PK | |
| name | VARCHAR(128) | NOT NULL, UNIQUE | 接入源名称，如 "生产环境 Prometheus" |
| token | CHAR(32) | NOT NULL, UNIQUE | webhook 凭证，URL 路径的一部分（32 位随机 hex） |
| description | VARCHAR(255) | NOT NULL DEFAULT '' | |
| enabled | BOOLEAN | NOT NULL DEFAULT TRUE | 禁用后 webhook 返回 403 |
| last_alert_at | TIMESTAMPTZ | NULL | 最近一次收到告警的时间（每批 webhook 更新一次）；接入源列表页据此直观判断「Alertmanager 的配置是否真的在推送」，是接入排障最常用的信号 |

说明：
- webhook 地址即 `POST /api/v1/alerts/webhook/:token`，token 放路径里而非 header，是因为 Alertmanager webhook_config 对自定义 header 支持麻烦
- token 可重置（rotate），旧 token 立即失效
- `last_alert_at` 的代价是每批 webhook 一次 UPDATE，量级可忽略，换来免查 alerts 表的健康度展示

## 5. templates — 消息模板（项目亮点）

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | BIGINT | PK | |
| name | VARCHAR(128) | NOT NULL | 模板名（同一 channel_type 下唯一） |
| channel_type | VARCHAR(16) | NOT NULL | 模板归属的渠道类型：feishu / dingtalk / wecom 三选一；内容即该渠道的消息 payload 模板 |
| content | TEXT | NOT NULL | 渠道消息 payload 的 Go template 文本，渲染结果就是直接发送的消息体（按渠道类型校验 JSON 与 msg_type/msgtype） |
| is_builtin | BOOLEAN | NOT NULL DEFAULT FALSE | TRUE=内置模板（只读可复制，不可改不可删） |
| remark | VARCHAR(255) | NOT NULL DEFAULT '' | |

**UNIQUE KEY `(channel_type, name)`**

说明：
- `is_builtin=TRUE` 的模板由迁移脚本写入（迁移 0005 起为按渠道类型编写的 payload 模板，4 个模板名 × 3 渠道 = 12 条），作为新用户的起点和兜底
- 渲染校验（能否用样例上下文渲染出本渠道的合法消息体 JSON）在应用层做，不入库

## 6. channels — 通知渠道（飞书/钉钉/企业微信机器人）

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | BIGINT | PK | |
| name | VARCHAR(128) | NOT NULL, UNIQUE | 渠道名 |
| type | VARCHAR(16) | NOT NULL DEFAULT 'feishu' | `feishu` / `dingtalk` / `wecom` |
| webhook_url | TEXT | NOT NULL DEFAULT '' | 机器人 webhook 完整地址，明文存储（URL 内含机器人 token，泄露=任何人可向群内发消息，属敏感凭证；接口返回脱敏） |
| secret | TEXT | NOT NULL DEFAULT '' | 加签密钥，明文存储；空串=未开启签名校验（wecom 无加签机制，恒为空） |
| keyword | VARCHAR(64) | NOT NULL DEFAULT '' | 机器人关键词安全设置（发送内容必须包含） |
| template_id | BIGINT | NULL, INDEX | 绑定模板；NULL=内置默认模板 |
| at_all | BOOLEAN | NOT NULL DEFAULT FALSE | 是否 @所有人（飞书/钉钉生效；企微 markdown 不支持 @，忽略） |
| extra | JSONB | NULL | 平台特有配置兜底：企业微信的 mentioned_list、钉钉的 atMobiles 等进这里，扩展新平台大概率零 DDL |
| enabled | BOOLEAN | NOT NULL DEFAULT TRUE | |

说明：
- webhook_url 与 secret 都是「可逆但必须保密」的凭证（secret 需还原用于签名，URL 需还原用于发请求），明文存储（取舍见「设计取舍备忘」）；接口返回脱敏（仅显示域名 + hook ID 尾 4 位），编辑时不重新提交则保持原值
- 消息形态（飞书互动卡片 / 钉钉与企微 markdown）不放渠道上——由所绑定的模板内容决定（模板按渠道类型编写完整消息 payload）
- **扩展性验证（新平台）**：type/webhook_url/secret/keyword/at_all 五列对飞书、钉钉、企业微信通用（飞书/钉钉有加签+关键词，企业微信仅 key）；平台特有配置（@指定人列表等）进 `extra` JSONB，不改表结构

## 7. routing_rules — 路由规则

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | BIGINT | PK | |
| source_id | BIGINT | NOT NULL, INDEX | 属于哪个接入源 |
| name | VARCHAR(128) | NOT NULL DEFAULT '' | 规则名（如 "生产 critical → 值班群"）；纯匹配条件在列表页可读性差，deliveries.rule_id 回查时也需要人类可读的标识 |
| priority | INT | NOT NULL DEFAULT 100 | 数字越小越先匹配 |
| match_labels | JSONB | NOT NULL | 匹配条件：`{"severity":"critical","namespace":"prod"}`，所有 key=value 全部命中才算匹配（AND） |
| channel_id | BIGINT | NOT NULL, INDEX | 命中后发送到该渠道 |
| continue_matching | BOOLEAN | NOT NULL DEFAULT FALSE | 命中后是否继续匹配后续规则（TRUE=一条告警可发多个渠道） |
| enabled | BOOLEAN | NOT NULL DEFAULT TRUE | |

说明：
- 「默认渠道」= `match_labels` 为 `{}` 且 priority 最大的规则，语义自然，不需要单独字段
- V1 仅等值匹配，在应用层对内存中的规则列表逐条比对；正则/运算符放 V2（`match_labels` JSONB 结构可平滑扩展为 `{"key":"severity","op":"regex","value":"crit.*"}`）
- JSONB 选型留出 DB 侧能力兜底：`match_labels @> '{"severity":"critical"}'` 包含查询语义与「AND 等值匹配」天然同构，未来如需按规则反查告警（`alerts.labels @> match_labels`）或按 label 做 ad-hoc 过滤，加 GIN 索引即可，不用改结构
- 不设 source-channel 独立绑定表，全部由规则表达，模型更简单

## 8. alerts — 告警记录

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | BIGINT | PK | |
| source_id | BIGINT | NOT NULL, INDEX | 来源 |
| fingerprint | CHAR(64) | NOT NULL | 本条告警指纹（取 Alertmanager 的 fingerprint，16 位 hex；缺失时由 labels 计算 SHA-256，64 位——列宽取 64 兼容两种情况） |
| content_hash | CHAR(64) | NOT NULL | 内容指纹：SHA-256(status + key 排序后的 labels + annotations)。去重判定（FR-1.3）直接比对此列，避免每次反序列化比对完整 JSON |
| status | VARCHAR(16) | NOT NULL | `firing` / `resolved` |
| alertname | VARCHAR(255) | NOT NULL DEFAULT '' | 冗余自 labels.alertname，便于筛选/排序 |
| severity | VARCHAR(32) | NOT NULL DEFAULT '' | 冗余自 labels.severity，便于筛选（列表页高频过滤字段） |
| labels | JSONB | NOT NULL | 完整 labels |
| annotations | JSONB | NOT NULL | 完整 annotations |
| starts_at | TIMESTAMPTZ | NOT NULL | |
| ends_at | TIMESTAMPTZ | NULL | |
| raw_payload | JSONB | NOT NULL | Alertmanager 原始 webhook 消息体（整组），排查与模板预览数据源 |
| disposition | VARCHAR(16) | NOT NULL DEFAULT 'delivered' | 处置结果：`delivered`（已分发到至少一个渠道）/ `deduped`（窗口内重复，未发送）/ `unmatched`（未命中任何路由规则）。「告警为什么没发出去」是排查最高频的问题，靠 join deliveries 推断既慢又不准，冗余一列直接可查 |
| received_at | TIMESTAMPTZ | NOT NULL, INDEX | 接收时间 |

**INDEX `(fingerprint, status)`**，**INDEX `(status, received_at)`**，**INDEX `(severity, received_at)`**

说明：
- 一次 webhook 的 `alerts[]` 拆成多行存储，`raw_payload` 每行冗余整组原文——存储换简单，模板预览随便取一条就能用
- `alertname`/`severity` 冗余是反范式索引优化：V1 列表页的固定过滤条件走普通 btree；如需按任意 label 做 ad-hoc 过滤（如 `labels @> '{"pod":"xxx"}'`），后续加 `GIN (labels jsonb_path_ops)` 即可，无需改表
- 去重实现：按 `(fingerprint, status)` 索引取窗口内最近一条，比对 `content_hash`，相同则新行标记 `disposition='deduped'`、不产生 deliveries；无需额外状态字段
- 数据清理：按保留天数（`FENGHUO_ALERT_RETENTION_DAYS`，默认 30 天）定时物理删除，deliveries 级联清理。流水表持续增长时可用 PG 声明式分区（按 received_at 月分区，`DROP PARTITION` 即完成清理，比 DELETE 快且无 vacuum 压力）——V1 数据量下非必需，列为演进选项

## 9. deliveries — 投递记录

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | BIGINT | PK | |
| alert_id | BIGINT | NOT NULL, INDEX | 关联 alerts.id |
| channel_id | BIGINT | NOT NULL, INDEX | 关联 channels.id（渠道删除后保留快照名） |
| channel_name | VARCHAR(128) | NOT NULL DEFAULT '' | 冗余渠道名快照，渠道被删后记录仍可读 |
| rule_id | BIGINT | NOT NULL DEFAULT 0 | 命中哪条规则（0=默认兜底），排查路由问题用 |
| trigger_type | VARCHAR(16) | NOT NULL DEFAULT 'auto' | `auto`（路由自动投递）/ `manual`（管理员手动重发产生的新记录，FR-2.6） |
| attempts | INT | NOT NULL DEFAULT 1 | 本条投递的尝试次数（有限重试，仅瞬时错误重试，FR-2.4） |
| status | VARCHAR(16) | NOT NULL | `success` / `failed` |
| http_status | INT | NOT NULL DEFAULT 0 | 渠道网关 HTTP 状态码 |
| response_code | INT | NOT NULL DEFAULT 0 | 渠道业务 code（0=成功） |
| response_msg | VARCHAR(512) | NOT NULL DEFAULT '' | 渠道返回消息或本地错误（超时/渲染失败等） |
| duration_ms | INT | NOT NULL DEFAULT 0 | 投递耗时 |
| rendered_payload | TEXT | NULL | 实际发送的消息体（模板渲染出的渠道 payload），排查模板问题用 |
| sent_at | TIMESTAMPTZ | NOT NULL | |

**INDEX `(status, sent_at)`**，**INDEX `(channel_id, sent_at)`**（告警列表按「渠道 + 时间范围」筛选时的 join 路径）

说明：
- 无重试队列、无状态机（已确认的边界：告警级重发是 Alertmanager 的职责）；有限重试在投递 goroutine 内原地完成，`attempts` 记录尝试次数，不产生额外行
- 手动重发（FR-2.6）不修改原记录，而是插入一条 `trigger_type='manual'` 的新行，沿用原 `alert_id`/`channel_id`/`rule_id`，保留完整排查痕迹；渠道已删除时应用层拒绝重发
- `rendered_payload` 只存一份最终发送体，TEXT 足够（各平台单消息上限 30KB 左右）

---

## 10. oauth_states — OAuth 登录 state（迁移 0006）

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| state | TEXT | PK | 随机 state，回调时一次性消费 |
| expires_at | TIMESTAMPTZ | NOT NULL, INDEX | 过期时间，后台任务定期清理 |

说明：CSRF state 从内存迁至数据库——多副本部署时签发与回调可能落在不同实例，内存态会导致登录间歇性失败

## 11. app_settings — 通用键值设置（迁移 0007）

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| key | TEXT | PK | 设置键，如 `jwt_secret` |
| value | TEXT | NOT NULL | 设置值 |
| updated_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | |

说明：首个用途是持久化启动时随机生成的 JWT 密钥（未显式配置 `FENGHUO_JWT_SECRET` 时），避免重启/多副本下各实例密钥不一致导致 token 失效

---

## 12. login_attempts — 登录限流（迁移 0008）

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| ip | TEXT | PK | 客户端 IP，每行一个 IP |
| fails | INT | NOT NULL DEFAULT 0 | 当前固定窗口内的失败计数 |
| window_start | TIMESTAMPTZ | NOT NULL | 固定窗口起点，`now - window_start >= window` 时窗口重置 |
| locked_until | TIMESTAMPTZ | NOT NULL DEFAULT '-infinity' | 锁定截止时间，`'-infinity'` 表示从未锁定 |

说明：
- 登录限流（NFR-1）从内存迁至数据库——多副本部署时内存计数会把防爆破阈值放大 N 倍且锁定状态不共享
- 语义为固定窗口（原内存版是滑动窗口，迁库后的常规取舍）：窗口内失败达上限则 `locked_until = now + lockTime` 并清零计数；同一 IP 的读改写由事务内 `SELECT ... FOR UPDATE` 行锁串行化，不同 IP 互不阻塞
- 过期行不删（体积按 distinct IP 增长，可接受），登录成功时删除对应行

---

## 13. ER 关系一览

```
users 1──────* oauth_identities
users 1──────* refresh_tokens

sources 1──────* routing_rules *──────1 channels *──────1 templates(可空)
sources 1──────* alerts 1──────* deliveries *──────1 channels
```

## 14. 设计取舍备忘

| 决策 | 取舍 |
|---|---|
| 逻辑外键，不建物理 FK | 便于告警/投递流水的批量清理，GORM 项目惯例；一致性由应用层保证 |
| 全部硬删除 | 管理类表删除前应用层校验引用（模板被渠道引用则拒绝删除）；流水表按保留期物理清理；不做软删除 |
| JSONB 字段大量使用 | labels/annotations/match_labels/extra 本就是半结构数据；JSONB 提供 `@>` 包含查询、GIN 索引、路径提取，远比 MySQL JSON 可用。高频过滤字段（severity/alertname）仍冗余为列走 btree，DB 能力不作为偷懒的理由 |
| 不用 ENUM | PG 虽有原生 ENUM，但 `ALTER TYPE ADD VALUE` 有事务限制；role/type/status/disposition 全部 VARCHAR，加值零 DDL，合法值由应用层校验 |
| PG 原生类型优先 | `BOOLEAN`（不用 TINYINT 模拟）、`TIMESTAMPTZ`（带时区，跨时区部署无歧义）、`GENERATED ALWAYS AS IDENTITY`（标准 SQL 自增，优于 SERIAL）——GORM 映射全部直接支持 |
| channel 删模板绑定不级联 | templates 删除时校验是否被渠道引用，被引用则拒绝删除 |
| channels.extra 预留 | 平台特有配置的统一出口，V2 企业微信 / V3 钉钉接入时大概率零 DDL |
| 渠道凭证明文存储 | webhook_url/secret 是「可逆但需保密」的凭证，曾用 AES-GCM 加密存储；但密钥与密文同库同机部署时加密收益有限，且引入密钥管理成本（轮换即全量数据失效），故改明文存储、移除 secret_key 设施；防护边界收敛为接口层脱敏返回（仅显示尾 4 位）与数据库访问控制 |
| alerts.content_hash | 去重判定（FR-1.3）落库为显式列：比对 64 字符定长字符串，免于反序列化比对完整 JSON，也让去重逻辑可测试、可审计 |
| deliveries.attempts / trigger_type | 两类重试语义分开承载：有限重试（FR-2.4）不新增行、只累加 `attempts`；手动重发（FR-2.6）新增 `trigger_type='manual'` 的行，原失败记录保持不动，排查痕迹完整 |
| alerts.disposition | 「为什么没发出去」的最高频答案（去重 / 无路由 / 已分发）冗余成一列，列表页免 join 可查；deliveries 仍保留逐渠道明细 |
| sources.last_alert_at | 接入源健康度的最简信号，一次 UPDATE 的代价换免查流水表的列表展示 |
| routing_rules.name | 纯 JSON 匹配条件在列表与投递排查中可读性差，规则名是人类可读的锚点 |
