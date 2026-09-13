# Fenghuo（烽火台）Helm Chart

在无内部 registry 的离线/内网 K8s 集群上部署 Fenghuo。

## 前置条件

- Kubernetes >= 1.25，Helm 3
- **外部 PostgreSQL 数据库**：chart 不再内置数据库，需提前建好库和用户（默认 `fenghuo`/`fenghuo`）
- 集群节点运行时为 containerd，镜像通过手动导入分发（`imagePullPolicy: IfNotPresent`）

## 1. 构建并分发镜像

```bash
docker build -t fenghuo:latest .
docker save fenghuo:latest -o fenghuo.tar
```

把 tar 包拷到**每个节点**（app 可能被调度到任意 worker），导入 containerd：

```bash
# 在每个节点上执行（注意 k8s.io 命名空间，否则 kubelet 看不到）
sudo ctr -n k8s.io images import fenghuo.tar
```

如果直接从需要认证的私有镜像仓库拉取，先在应用 namespace 创建拉取凭证：

```bash
kubectl create secret docker-registry harbor-pull-secret -n fenghuo \
  --docker-server=harbor.example.com \
  --docker-username=<用户名> \
  --docker-password=<密码>
```

然后在 values 中引用：

```yaml
imagePullSecrets:
  - name: harbor-pull-secret
```

## 2. 密钥管理

应用必需的密钥：`FENGHUO_JWT_SECRET`、数据库密码；开启飞书 OAuth 登录时还需要
飞书 App Secret。

所有敏感信息只通过外部 Secret 提供，不经过 values——chart 不会生成 Secret，
values 里也没有任何可填密码的字段，可以避免明文落入 Helm release 历史或
误提交进 git。

预先创建 Secret，key 名约定如下：

```bash
kubectl create secret generic fenghuo-secrets -n fenghuo \
  --from-literal=jwt-secret=<随机串> \
  --from-literal=database-password=<数据库密码>
# 开启飞书 OAuth 时追加：
#   --from-literal=feishu-app-secret=<飞书 App Secret>
```

安装时通过 `secrets.existingSecret` 引用它（必填，留空时 helm 直接报错）：

```bash
helm install fenghuo ./helm/fenghuo \
  -n fenghuo --create-namespace \
  --set database.host=<数据库地址> \
  --set secrets.existingSecret=fenghuo-secrets
```

> `database.host` 和 `secrets.existingSecret` 均为必填，未提供时 helm 会直接报错。
> chart 不提供默认弱密钥。

## 3. 验证

```bash
helm list -n fenghuo
kubectl get pods -n fenghuo
kubectl port-forward -n fenghuo svc/fenghuo 8080:8080   # 访问 http://localhost:8080
```

初始管理员通过首次访问 Web 界面按引导设置（FR-5.1），无需在 values 中预置账号。

## 4. 可选配置

### 通过 Higress Ingress 暴露

```bash
helm upgrade fenghuo ./helm/fenghuo -n fenghuo \
  --set database.host=<数据库地址> \
  --set secrets.existingSecret=fenghuo-secrets \
  --set ingress.enabled=true --set ingress.host=fenghuo.local \
  --set ingress.path=/fenghuo \
  --set rootUrl=http://fenghuo.local/fenghuo/
```

### 飞书 OAuth 登录

应用使用以下环境变量启用飞书登录：

- `FENGHUO_OAUTH_ENABLED`
- `FENGHUO_OAUTH_FEISHU_APP_ID`
- `FENGHUO_OAUTH_FEISHU_APP_SECRET`
- `FENGHUO_OAUTH_ALLOWED_EMAILS`（逗号分隔的邮箱白名单，留空不限制）
- `FENGHUO_OAUTH_AUTO_CREATE_USER`（首次登录自动创建本地用户）

为 Secret 增加 `feishu-app-secret` key，然后配置：

```yaml
oauth:
  enabled: true
  feishuAppId: "cli_xxxxxxxxxxxxx"
  allowedEmails:
    - user1@example.com
    - user2@example.com
  autoCreateUser: true

secrets:
  existingSecret: fenghuo-secrets
```

飞书开放平台的重定向 URL 为：

```text
<rootUrl>api/auth/oauth/feishu/callback
```

例如 `rootUrl` 为 `https://example.com/fenghuo/` 时，重定向 URL 是
`https://example.com/fenghuo/api/auth/oauth/feishu/callback`。

### 多副本

`replicaCount` 大于 1 时需要共享的外部数据库——本 chart 只支持外部数据库，该前提已天然
满足，直接 `--set replicaCount=2` 即可。

### 最小 values 示例

```yaml
database:
  host: "postgres.example.internal"
secrets:
  existingSecret: "fenghuo-secrets"
```

### 常用 values 说明

| 参数 | 说明 | 默认值 |
| --- | --- | --- |
| `image.repository` / `image.tag` | 应用镜像 | `fenghuo` / `latest` |
| `imagePullSecrets` | 私有仓库拉取凭证列表 | `[]` |
| `replicaCount` | 副本数（>1 需外部数据库，已满足） | `1` |
| `rootUrl` | 外部访问地址，须以 `/` 结尾 | `http://localhost:8080/` |
| `timezone` | 容器时区（TZ），影响告警时间渲染与「今日/本周」统计口径 | `Asia/Shanghai` |
| `database.host` | 外部 PostgreSQL 地址（必填） | `""` |
| `secrets.existingSecret` | 预先创建的应用 Secret 名称（必填） | `""` |
| `oauth.enabled` | 飞书 OAuth 登录开关 | `false` |
| `ingress.enabled` / `ingress.host` / `ingress.path` | Ingress 暴露 | `false` / `fenghuo.local` / `/` |
| `alert.dedupWindow` / `alert.retentionDays` | 告警去重窗口 / 保留天数 | `5m` / `30` |
| `resources` | 容器资源限制 | `{}` |

### 其他 values

见 [values.yaml](./values.yaml)，所有应用配置与 `config.yaml` 中的 `FENGHUO_*`
环境变量一一对应。

## 卸载

```bash
helm uninstall fenghuo -n fenghuo
# chart 不创建任何 PVC；外部 Secret 由用户管理，卸载时不会被删除
```
