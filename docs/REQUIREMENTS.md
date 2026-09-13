# 烽火台（Fenghuo） 需求文档（V1）

> 版本：v0.2
> 日期：2026-08-15
> 状态：实现中——需求项复选框标注实现进度（`- [x]` 已完成 / `- [ ]` 未完成），随代码迭代更新

---

## 1. 项目背景与痛点

### 1.1 背景

Prometheus 生态中，Alertmanager 是事实标准的告警路由/通知组件，但它对接国内 IM（飞书、企业微信、钉钉）的体验并不好：

- **核心痛点**：飞书自定义机器人开启「签名校验」后，需要按 `timestamp + "\n" + secret` 做 HMAC-SHA256 签名，市面上主流的告警转发器（如 prometheus-webhook-dingtalk 及部分开源 webhook 工具）**不支持或支持不完整**，导致安全合规要求高的团队（机器人必须开启签名校验）无法直接使用。
- Alertmanager 自带的 webhook 消息体是原始 JSON，不经过二次封装直接发给 IM 机器人，可读性差，缺少告警详情、分级展示、恢复通知美化等能力。
- 缺少统一管理界面：webhook URL、签名密钥、路由规则等散落在各个配置文件里，无法可视化维护。

### 1.2 项目定位

烽火台（Fenghuo） 是一个**告警转发中台**：接收 Alertmanager 的 webhook 告警，经过模板封装后，转发到各类 IM 机器人。V1 支持飞书自定义机器人（含签名校验）、钉钉自定义机器人（含加签）和企业微信群机器人。

### 1.3 目标用户

- 运维 / SRE 工程师：需要把 Prometheus 告警可靠地投递到 IM 群。
- 有安全合规要求的团队：IM 机器人必须开启签名校验/加签。

---

## 2. 项目目标与范围

### 2.1 V1 目标（Must Have）

1. 接收 Alertmanager webhook 告警（标准 v4 消息格式）。
2. 告警模板化封装（按渠道类型编写的消息 payload Go template），转发至**飞书 / 钉钉 / 企业微信机器人**，飞书、钉钉**完整支持签名校验（加签）模式**，同时兼容仅关键词/无安全设置模式。
3. 提供 Web 管理界面：机器人管理、告警记录、路由规则、用户登录。
4. 支持通过环境变量配置 **Root URL**（类似 Grafana 的 `GF_SERVER_ROOT_URL`），便于反向代理/子路径部署。
5. 支持 **飞书 OAuth2 授权登录**（扫码/网页授权）+ 本地账号 + JWT 认证。

### 2.2 后续版本目标（Nice to Have）

- 企业微信机器人、钉钉机器人（钉钉同样有加签机制，架构预留）。
- 告警静默、抑制、聚合规则。
- 多 Alertmanager 实例接入（按 access key 区分来源）。
- 告警升级（未确认时电话/短信通知）。
- 飞书 interactive card 按钮（认领、静默）。

### 2.3 明确不做（V1 范围外）

- 不替代 Alertmanager 本身（不做告警规则计算）。
- 不做指标采集、图表展示（与 Grafana 边界划分清楚）。
- 不支持短信/电话/邮件渠道。

---

## 3. 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.25+，Gin（HTTP 框架），GORM（ORM） |
| 前端 | React 18 + TypeScript + Vite，Ant Design（建议） |
| 数据库 | PostgreSQL 18（最低兼容 16） |
| 认证 | JWT（access + refresh token），OAuth2 第三方登录 |
| 部署 | 仅 Docker / Helm：Docker 镜像（前端静态资源经 `go:embed` 内嵌），Helm chart 部署 K8s；不提供裸二进制部署方式 |

---

## 4. 功能需求

### 4.1 告警接收（Webhook Receiver）

- [x] **FR-1.1** 提供 Alertmanager webhook 接收端点：

```
POST /api/v1/alerts/webhook/:token
```

- `:token` 为接入凭证，关联到一个「接入源（Source）」，一个接入源可绑定多个转发渠道。
- 消息体兼容 Alertmanager v4 webhook 格式（`version: "4"`，含 `alerts[]`、`groupLabels`、`commonLabels`、`externalURL` 等）。
- 接收后立即异步处理，HTTP 响应不阻塞在 IM 投递上（建议 2s 内返回 200）。

- [x] **FR-1.2** 告警字段解析：提取 `status`（firing/resolved）、`labels`、`annotations`、`startsAt`、`endsAt`、`fingerprint`，结构化入库。

- [x] **FR-1.3** 幂等与去重：同一 `fingerprint + status` 且内容未变化的告警在去重窗口内重复推送时，仅记录不重复发送（窗口由 `FENGHUO_ALERT_DEDUP_WINDOW` 配置，默认 5 分钟）。

- 「内容未变化」以对 `status + labels + annotations`（key 排序后）计算 SHA-256 得到的 `content_hash` 判定，入库时一并存储，避免每次比对完整 JSON。
- 被去重的告警正常入库但 `disposition` 标记为 `deduped`，不产生投递记录；未命中任何路由规则的标记为 `unmatched`。告警列表可据此区分「没发出去」的原因。

### 4.2 通知渠道（飞书 / 钉钉 / 企业微信机器人）— V1 核心

- [x] **FR-2.1** 机器人配置（CRUD），渠道类型支持飞书自定义机器人（feishu）、钉钉自定义机器人（dingtalk）、企业微信群机器人（wecom），字段包括：

| 字段 | 说明 |
|---|---|
| 名称 | 展示用 |
| 渠道类型 | feishu / dingtalk / wecom |
| Webhook URL | 飞书 `https://open.feishu.cn/open-apis/bot/v2/hook/xxx`；钉钉 `https://oapi.dingtalk.com/robot/send?access_token=xxx`；企微 `https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx` |
| 签名密钥 Secret | 可选；飞书/钉钉填写后启用加签；企微无加签机制，忽略 |
| 关键词 | 可选；机器人关键词安全设置时，发送内容必须包含该关键词 |
| 绑定模板 | 可选；未绑定时使用内置默认模板 |
| 是否启用 | 开关 |

> 注：消息形态（飞书互动卡片 / 钉钉与企微 markdown 消息）**不在渠道上配置**，由模板内容决定——模板是按渠道类型编写的完整消息 payload 的 Go template，渲染结果就是直接发送的消息体。Webhook URL 与签名密钥在接口返回值中均脱敏展示（仅显示尾部 4 位），编辑时不重新提交则保留原值。

- [x] **FR-2.2 签名校验（核心痛点）**：

- 当配置了 Secret 时，按飞书官方算法生成签名：
  - `stringToSign = timestamp + "\n" + secret`
  - `sign = Base64(HMAC-SHA256(key = stringToSign, data = ""))`（即以 `timestamp + "\n" + secret` 为 HMAC 密钥、空串为消息体）
- 请求体携带 `timestamp` 与 `sign` 字段。
- 钉钉加签按其官方算法：`sign = Base64(HMAC-SHA256(key = secret, data = timestamp(毫秒) + "\n" + secret))`，URL 编码后拼到 webhook URL 的 `timestamp`/`sign` 参数。
- 签名密钥与 Webhook URL 在数据库中**明文存储**：Webhook URL 内含机器人 token，泄露即等于任何人可向群内发消息，敏感度与 Secret 同级；接口返回值均脱敏（仅显示尾部 4 位），不明文返回前端。

- [x] **FR-2.3 消息模板封装（V1 核心亮点）**：

- 内置若干基础模板（critical/warning/resolved、纯文本），开箱即用。
- **支持用户自定义模板（Go `text/template` 语法，内容即渠道消息 payload）——本项目差异化亮点**：
  - 模板管理（CRUD）：名称、渠道类型、模板内容、备注；模板按渠道类型归属（`channel_type` 为 feishu/dingtalk/wecom 三选一），一个渠道只能绑定同类型的模板，未绑定时用该渠道类型的内置默认模板。
  - 模板内容是按渠道类型编写的完整消息 payload 的 Go template，渲染结果直接作为发送给机器人的消息体：飞书要求合法 JSON 且 `msg_type` ∈ {text, post, interactive, image, share_chat}；钉钉/企微要求合法 JSON 且 `msgtype` ∈ {text, markdown}。插值嵌入 JSON 字符串时用 `jesc` 函数转义。保存与预览时按渠道类型校验渲染结果。
  - 模板上下文：暴露完整告警数据结构（`{{ .Status }}`、`{{ .Alerts }}`、`{{ .CommonLabels }}`、`{{ range .Alerts }}...{{ end }}` 等）+ 系统变量（Root URL、接入源名称）。
  - 内置函数：severity 颜色映射、时间格式化（时区可配）、label 取值/截断等；引入 Sprig 函数库。
  - **在线预览**：模板编辑页输入/自动带入最近一条真实告警 JSON，按模板的渠道类型实时渲染预览消息体 payload（飞书卡片另附本地模拟的卡片效果）；语法错误与渲染错误即时提示。
  - 模板保存时用样例上下文试渲染，失败时拒绝保存并给出原因。
- 卡片内告警详情跳转链接使用 Root URL 拼接。

- [x] **FR-2.4 发送语义（有限重试，无重试队列）**：

- **职责边界：告警生命周期管理（分组、抑制、静默、告警级重发）是 Alertmanager 的职责，本项目只做「通知编排 + 渠道发送」**。
- 收到 webhook 后按路由规则分发到渠道，**即时发送**：HTTP 请求级超时（默认 10s）。
- **有限重试**：投递失败时，仅对瞬时错误（网络错误、超时、HTTP 5xx、IM 平台限频）在投递 goroutine 内原地重试，默认最多重试 2 次（退避 1s、3s，总耗时有界）；次数由 `FENGHUO_CHANNEL_RETRY_MAX` 配置，`0` 表示关闭。**明确失败的业务错误（签名错误、关键词缺失、机器人被移除等）不重试**——重试不会成功，交给人工排查。
- 注意：Alertmanager 的 webhook 重发只在 Fenghuo 返回非 2xx 时触发，而 Fenghuo 接收后即返回 200，因此 **IM 投递失败没有外部兜底**——本地有限重试 + editor/admin 手动重发（FR-2.6）是仅有的两层保险。
- 投递结果（成功/失败、尝试次数、渠道返回码与错误信息、耗时）完整记录到 `deliveries` 表，供页面排查；不做后台重试队列、不做状态机。
- 渠道侧明确报错（如签名错误、关键词缺失、机器人被移除）时，投递记录中给出人类可读的失败原因提示。

- [x] **FR-2.5** 测试发送：渠道配置页提供「发送测试消息」按钮，立即验证 URL + 签名是否正确。

- [x] **FR-2.6 失败投递管理（人工兜底）**：

- 失败投递列表（editor / admin）：按时间范围、渠道筛选，展示失败原因、尝试次数、关联告警。
- **手动重发**：editor / admin 可对单条失败投递执行重发——以**当前**渠道配置与模板重新渲染发送（原模板/渠道配置可能正是失败原因，重发应使用修复后的配置）；渠道已删除则拒绝重发并提示。
- 手动重发产生一条新的 `deliveries` 记录（`trigger_type='manual'`），不修改原失败记录，保留完整排查痕迹。
- viewer 只读可见投递记录，无重发操作入口。

### 4.3 路由规则（Routing）

- [x] **FR-3.1** 接入源（Source）→ 渠道（Channel）多对多绑定。

- [x] **FR-3.2** 简单路由规则（V1）：按 label 匹配（如 `severity=critical`、`namespace=prod`）将告警分发到指定渠道；支持「默认渠道」兜底；多条规则按优先级排序，命中即发送，可配置是否继续匹配后续规则。

### 4.4 告警记录（Alert History）

- [x] **FR-4.1** 告警列表：状态、级别、名称、实例、时间范围、渠道筛选；分页。

- [x] **FR-4.2** 告警详情：原始 JSON、渲染后的消息内容、投递记录（渠道、时间、结果、失败原因、重试次数）。

### 4.5 认证与用户

- [x] **FR-5.1** 本地账号登录：用户名 + 密码（bcrypt），首次启动后通过 Web 界面引导创建管理员（`POST /api/v1/setup`，完成后该接口关闭）。

- [x] **FR-5.2 JWT 认证**：

- Access Token（默认 2h）+ Refresh Token（默认 7d）。
- V1 统一通过 `Authorization: Bearer` 头传递 token，不使用 cookie——纯 Bearer 方案天然规避 CSRF（见 NFR-1），前端实现也更简单。
- Refresh Token 存库可吊销；支持登出（吊销对应 refresh token）。
- 除 `/api/v1/alerts/webhook/:token`、登录/ OAuth 回调、健康检查外，所有 API 需鉴权。

- [x] **FR-5.3 飞书 OAuth2 授权登录（V1 唯一 OAuth Provider）**：

- 仅支持飞书开放平台 OAuth（网页授权 / 扫码登录），不做 GitHub/通用 OIDC（后续按需扩展，代码按 provider 接口抽象预留）。
- 需要用户在飞书开放平台创建「企业自建应用」，配置重定向 URL。
- 配置项（环境变量）：
  ```
  FENGHUO_OAUTH_ENABLED=true
  FENGHUO_OAUTH_FEISHU_APP_ID=cli_xxx
  FENGHUO_OAUTH_FEISHU_APP_SECRET=xxx
  FENGHUO_OAUTH_AUTO_CREATE_USER=true        # 首次登录自动创建本地账号
  FENGHUO_OAUTH_ALLOWED_EMAILS=a@x.com,b@x.com   # 可选白名单，空则不限制
  ```
- 授权流程：前端跳转飞书授权页 → 回调拿 `code` → 后端用 `app_access_token` 换 `user_access_token` → 拉取用户信息（open_id、姓名、邮箱、头像）。
- 回调地址基于 Root URL 自动拼接：`{ROOT_URL}/api/auth/oauth/feishu/callback`，需在飞书应用后台配置一致。
- 首次 OAuth 登录自动创建本地用户并绑定 `open_id`（默认 viewer 角色，admin 可在用户管理中提升）。
- `FENGHUO_OAUTH_AUTO_CREATE_USER=false` 时仅允许已绑定 `open_id` 的已有用户登录，其余拒绝。
- 用户表存储飞书头像 URL 与姓名，展示在界面右上角。

- [x] **FR-5.4 角色**：三级角色（对齐 Grafana 的 Viewer/Editor/Admin 语义）：

| 能力 | viewer | editor | admin |
|---|---|---|---|
| 告警/投递记录、仪表盘查看；修改本人密码与个人信息 | ✅ | ✅ | ✅ |
| 接入源/渠道/模板/路由规则 CRUD、接入源 token 轮换 | | ✅ | ✅ |
| 渠道测试发送、失败投递手动重发（FR-2.6） | | ✅ | ✅ |
| 用户管理（创建、改角色、启用/禁用、删除） | | | ✅ |

- editor 定位为「可信运维」：接口返回的凭证均脱敏，但 editor 可使用凭证（测试发送/手动重发）并控制告警流向，部署方授权时需知悉。
- OAuth 自动创建的用户默认 viewer，由 admin 在用户管理中按需提升。

### 4.6 Root URL 配置（类 Grafana）

- [x] **FR-6.1** 环境变量：

```
FENGHUO_SERVER_ROOT_URL=https://alerts.example.com/        # 完整外部访问地址
FENGHUO_SERVER_HTTP_ADDR=0.0.0.0:8080                       # 监听地址
```

- [x] **FR-6.2** 行为要求（对齐 Grafana `GF_SERVER_ROOT_URL` 语义）：

- 系统内生成的所有绝对链接（OAuth 回调、告警详情跳转、邮件/卡片中的链接）均基于 Root URL 拼接。
- 支持子路径部署，如 `FENGHUO_SERVER_ROOT_URL=https://example.com/freealertflow/`，前端路由与 API 均带该前缀正常工作。
- 前端构建产物通过后端注入 runtime 配置（`window.__FENGHUO_CONFIG__`），避免前端为不同 Root URL 重新构建。

### 4.7 系统配置总览（环境变量）

| 变量 | 默认值 | 说明 |
|---|---|---|
| `FENGHUO_SERVER_HTTP_ADDR` | `:8080` | 监听地址 |
| `FENGHUO_SERVER_ROOT_URL` | `http://localhost:8080/` | 外部访问根地址 |
| `FENGHUO_SERVER_TRUSTED_PROXIES` | —（不信任任何代理头） | 可信反向代理 IP/CIDR，逗号分隔；部署在反代之后时必须配置，否则按 IP 的登录限流可被伪造 XFF 绕过（NFR-1） |
| `FENGHUO_DATABASE_HOST` / `FENGHUO_DATABASE_PORT` | `localhost` / `5432` | PostgreSQL 地址 |
| `FENGHUO_DATABASE_USER` / `FENGHUO_DATABASE_PASSWORD` / `FENGHUO_DATABASE_DBNAME` | — | PostgreSQL 账号、密码、库名（user/dbname 必填） |
| `FENGHUO_DATABASE_SSLMODE` | `disable` | PostgreSQL SSL 模式 |
| `FENGHUO_JWT_SECRET` | 随机生成（启动警告） | JWT 签名密钥 |
| `FENGHUO_OAUTH_*` | — | OAuth2 配置，见 4.5 |
| `FENGHUO_LOG_LEVEL` | `info` | 日志级别 |
| `FENGHUO_ALERT_DEDUP_WINDOW` | `5m` | 告警去重窗口（FR-1.3），`0` 表示关闭去重 |
| `FENGHUO_ALERT_RETENTION_DAYS` | `30` | 告警与投递记录保留天数，到期物理清理 |
| `FENGHUO_CHANNEL_HTTP_TIMEOUT` | `10s` | 渠道发送的 HTTP 请求超时（FR-2.4） |
| `FENGHUO_CHANNEL_RETRY_MAX` | `2` | 投递失败后的重试次数（仅瞬时错误，退避 1s/3s，FR-2.4），`0` 表示关闭 |
| `FENGHUO_JWT_ACCESS_TTL` / `FENGHUO_JWT_REFRESH_TTL` | `2h` / `7d` | JWT 有效期 |

配置优先级：环境变量 > 配置文件（可选 `config.yaml`）> 默认值。

---

## 5. 非功能需求

**NFR-1 安全**

- 签名密钥、OAuth secret、JWT secret 不落日志、不明文返回前端。
- 所有写接口 CSRF 防护（或纯 Bearer Token 方案规避）。
- Webhook token 使用 32 位以上随机串，可重置。
- 密码 bcrypt（cost ≥ 10）。
- 登录限流：同一 IP 5 次/分钟失败锁定 10 分钟。

**NFR-2 性能**

- 告警接收接口 P99 < 200ms：接收 → 解析 → 入库后立即返回 200，路由分发与渠道发送在后台 goroutine 中即时执行（带超时、有限重试，见 FR-2.4）。进程重启导致的在途丢失由 Alertmanager 的 webhook 重发机制兜底（仅限 Fenghuo 未返回 200 的场景；已返回 200 后的 IM 投递失败由有限重试与手动重发兜底）。接入源绑定多渠道时并行发送。
- 单实例支撑 100 告警/秒的接收峰值。
- 投递耗时与成功率通过 `/metrics` 暴露。

**NFR-3 可观测性**

- 结构化日志（zap 或 slog）。
- `/healthz`、`/readyz` 健康检查。
- 暴露 `/metrics`（Prometheus）：告警接收数、去重抑制数（`deduped`/`unmatched`）、按渠道与结果细分的投递成功/失败数、投递耗时直方图、手动重发数。不做重试队列，故不提供队列深度类指标；重试情况由 `deliveries.attempts` 统计。

**NFR-4 可部署性**

- 仅支持两种部署方式：Docker 镜像（含 docker compose）与 Helm chart（K8s）；不发布、不文档化裸二进制部署（本地开发 `go run` 不受限）。
- 提供 Dockerfile（多阶段构建，前端产物内嵌）。
- 提供 docker-compose.yml（app + PostgreSQL 18）一键起 demo。
- 提供 Helm chart（`helm/fenghuo`）：可配置镜像、数据库连接、Ingress、资源限制。
- 数据库迁移自动执行（gormigrate 或 golang-migrate）。

**NFR-5 兼容性**

- PostgreSQL 16+（推荐 18）；浏览器支持 Chrome/Edge 最近两个大版本。

---

## 6. 数据模型（草案）

> 字段级定义、索引与取舍说明以 [DATABASE_DESIGN.md](./DATABASE_DESIGN.md) 为准，此处仅为实体概览。

```
users               (id, username, password_hash, name, email, avatar_url,
                     role, status, last_login_at, ...)
oauth_identities    (id, user_id FK, provider /* feishu */, provider_user_id /* open_id */,
                     provider_union_id, extra JSON, created_at)
refresh_tokens      (id, user_id FK, token_hash, expires_at, revoked,
                     replaced_by, client_ip, user_agent, created_at)
sources             (id, name, token, description, enabled, last_alert_at, created_at)
channels            (id, name, type /* feishu|wecom|dingtalk */, webhook_url_encrypted,
                     secret_encrypted, keyword, template_id FK NULL,
                     at_all, extra JSON, enabled, ...)
routing_rules       (id, source_id FK, name, priority, match_labels JSON,
                     channel_id FK, continue_matching, enabled)
alerts              (id, source_id FK, fingerprint, content_hash, status, alertname,
                     severity, labels JSON, annotations JSON, starts_at, ends_at,
                     raw_payload JSON, disposition, received_at)
deliveries          (id, alert_id FK, channel_id FK, channel_name, rule_id,
                     trigger_type /* auto|manual */, attempts,
                     status /* success|failed */, http_status, response_code,
                     response_msg, duration_ms, rendered_payload, sent_at)
templates           (id, name, channel_type /* feishu|dingtalk|wecom */, content TEXT,
                     is_builtin, remark, created_at, updated_at)
```

---

## 7. API 概览（草案）

```
# 公开
POST   /api/v1/alerts/webhook/:token      告警接收
POST   /api/auth/login                    本地登录
POST   /api/auth/refresh                  刷新 token
POST   /api/auth/logout                   登出（吊销 refresh token）
GET    /api/auth/oauth/feishu           发起飞书授权登录
GET    /api/auth/oauth/feishu/callback  飞书授权回调
GET    /healthz /readyz /metrics

# 鉴权（JWT）；角色矩阵见 FR-5.4：GET 类接口 viewer 起，写操作 editor 起，用户管理 admin 专属
GET/POST/PUT/DELETE  /api/v1/sources[/:id]            查 viewer / 写 editor
POST   /api/v1/sources/:id/rotate-token               editor
GET/POST/PUT/DELETE  /api/v1/channels[/:id]           查 viewer / 写 editor（URL/secret 脱敏返回）
POST   /api/v1/channels/:id/test                      测试发送（editor）
GET/POST/PUT/DELETE  /api/v1/templates[/:id]          查 viewer / 写 editor
POST   /api/v1/templates/preview                      模板渲染预览（用样例/真实告警 JSON，editor）
GET/POST/PUT/DELETE  /api/v1/rules[/:id]              查 viewer / 写 editor
GET    /api/v1/alerts                                 告警列表
GET    /api/v1/alerts/:id                             告警详情（含投递记录）
GET    /api/v1/deliveries?status=failed               投递记录列表（editor/admin，可按失败筛选）
POST   /api/v1/deliveries/:id/resend                  手动重发失败投递（editor/admin，FR-2.6）
GET    /api/v1/users/me                               当前用户信息
PUT    /api/v1/users/me/password                      修改密码（成功后吊销本人全部 refresh token）
GET/PUT/DELETE  /api/v1/users[/:id]                   用户管理（admin 专属：列表、改角色、启用/禁用、删除）
GET    /api/v1/system/info                            版本、Root URL、OAuth 开关等（供前端 runtime 配置）
```

---

## 8. 页面规划（前端）

1. **登录页**：本地账号 + OAuth 登录按钮（按后端返回的 provider 动态渲染）。
2. **仪表盘**：今日告警数、投递成功率、失败 Top 渠道（V1 简版）。
3. **告警记录**：筛选 + 分页 + 详情抽屉；editor/admin 可见的「失败投递」视图：失败原因查看 + 手动重发（FR-2.6）。
4. **接入源管理**：列表、创建、token 复制/重置、Alertmanager 配置示例生成。
5. **渠道管理**：飞书/钉钉/企业微信机器人配置表单（含 Secret 加签开关）、测试发送。
6. **模板管理（亮点页）**：内置模板列表、自定义模板编辑器（代码高亮 + 实时渲染预览 + 真实告警样例数据）、模板与渠道绑定。
7. **路由规则**：规则列表、label 匹配编辑器、拖拽排序（V1 可简化为优先级数字）。
8. **系统设置**：个人信息、修改密码、（admin）用户管理。

---

## 9. 里程碑建议

| 阶段 | 内容 | 验收标准 | 状态 |
|---|---|---|---|
| M1 骨架 | 工程脚手架、配置加载、DB 迁移、JWT 登录 | 能登录、能连库 | ✅ 代码完成，待实机验收 |
| M2 核心链路 | Webhook 接收 → IM 发送（飞书/钉钉/企微，含加签）→ 记录入库 | 开启签名校验的机器人能收到告警消息 | ✅ 代码完成，待实机验收 |
| M3 管理界面 | 渠道/接入源/告警记录页面、模板管理（含在线预览）、路由规则 | 全流程 UI 可操作 | ✅ 代码完成，待实机验收 |
| M4 OAuth + Root URL | OAuth2 登录、Root URL 子路径部署 | 子路径反向代理下功能完整可用 | ✅ 代码完成，待实机验收 |
| M5 打磨 | 测试、Docker、文档、demo compose | `docker compose up` 一键可用 | ⬜ 未开始 |

---

## 10. 风险与待确认事项

| # | 事项 | 说明 |
|---|---|---|
| 1 | ~~OAuth Provider 范围~~ | ✅ 已确认：V1 仅飞书 OAuth 授权登录，代码按 provider 接口抽象预留扩展 |
| 2 | ~~消息模板自定义程度~~ | ✅ 已确认：自定义 Go template 是 V1 核心亮点，含模板管理 + 在线渲染预览；内置模板仅作基础兜底 |
| 3 | ~~异步投递实现~~ | ✅ 已确认：不做重试队列/状态机；投递层有限重试（默认 2 次，仅瞬时错误）+ admin 手动重发（FR-2.6）双层兜底，告警级重发仍归 Alertmanager（不与其抢职责） |
| 4 | 多租户 | 是否需要团队/命名空间隔离？V1 建议不做，按单团队设计 |
| 5 | 企业微信/钉钉 | 数据结构已预留 `type` 字段，V2 接入时确认接口抽象是否需调整 |
