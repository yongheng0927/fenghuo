{{/*
扩展 chart 名称（不含 version）
*/}}
{{- define "fenghuo.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
完整 release 名称
*/}}
{{- define "fenghuo.fullname" -}}
{{- if contains .Chart.Name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "fenghuo.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
通用标签
*/}}
{{- define "fenghuo.labels" -}}
helm.sh/chart: {{ include "fenghuo.chart" . }}
{{ include "fenghuo.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
{{- end }}

{{- define "fenghuo.selectorLabels" -}}
app.kubernetes.io/name: {{ include "fenghuo.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
应用 Secret 名称：必须指向预先创建好的 Secret，
须包含 key：jwt-secret / database-password（开启飞书 OAuth 时还须 feishu-app-secret）
*/}}
{{- define "fenghuo.secretName" -}}
{{- required "secrets.existingSecret 必填：请预先创建包含 jwt-secret / database-password 的 Secret，并用 --set secrets.existingSecret=<名称> 引用" .Values.secrets.existingSecret -}}
{{- end }}

{{/*
URL 路径前缀（子路径部署）：从 rootUrl 提取并去掉尾斜杠，
如 "https://example.com/fenghuo/" -> "/fenghuo"，根路径部署时为""。
探针等容器内访问路径都要加此前缀（后端所有路由挂在 base path 下）
*/}}
{{- define "fenghuo.basePath" -}}
{{- get (urlParse .Values.rootUrl) "path" | default "" | trimSuffix "/" -}}
{{- end }}
