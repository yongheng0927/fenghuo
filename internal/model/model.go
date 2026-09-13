// Package model 定义与 migrations/0001_init.up.sql 对应的 GORM 模型
// 所有关联仅是逻辑上的（无物理外键，见数据库设计 §11），
// 因此不使用 gorm 的 foreignKey 标签
package model

import (
	"encoding/json"
	"time"
)

// 角色取值（VARCHAR 语义枚举，在应用层校验）
const (
	RoleViewer = "viewer"
	RoleEditor = "editor"
	RoleAdmin  = "admin"
)

// 通知渠道类型取值（channels.type，VARCHAR 语义枚举，在应用层校验）
const (
	ChannelTypeFeishu   = "feishu"   // 飞书自定义机器人
	ChannelTypeDingTalk = "dingtalk" // 钉钉自定义机器人
	ChannelTypeWeCom    = "wecom"    // 企业微信群机器人
)

// User 是本地账号（users 表）
type User struct {
	ID           int64      `gorm:"primaryKey;autoIncrement"`
	Username     string     `gorm:"column:username"`
	PasswordHash *string    `gorm:"column:password_hash"` // 纯 OAuth 用户为 NULL
	Name         string     `gorm:"column:name"`
	Email        string     `gorm:"column:email"`
	AvatarURL    string     `gorm:"column:avatar_url"`
	Role         string     `gorm:"column:role"`
	Enabled      bool       `gorm:"column:enabled"`
	IsBootstrap  bool       `gorm:"column:is_bootstrap"` // 引导创建的初始管理员（受保护，全表至多一行）
	LastLoginAt  *time.Time `gorm:"column:last_login_at"`
	// 密码最近修改时间；改密前签发的 access token 一律失效（迁移 0011）。
	// 纯 OAuth 用户为 NULL（无本地密码，不参与该校验）
	PasswordChangedAt *time.Time `gorm:"column:password_changed_at"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (User) TableName() string { return "users" }

// OAuthIdentity 将用户绑定到第三方账号（oauth_identities 表）
type OAuthIdentity struct {
	ID              int64            `gorm:"primaryKey;autoIncrement"`
	UserID          int64            `gorm:"column:user_id"`
	Provider        string           `gorm:"column:provider"` // V1 中为 "feishu"
	ProviderUserID  string           `gorm:"column:provider_user_id"`
	ProviderUnionID string           `gorm:"column:provider_union_id"`
	Extra           *json.RawMessage `gorm:"column:extra;type:jsonb"`
	CreatedAt       time.Time        `gorm:"column:created_at"`
	UpdatedAt       time.Time        `gorm:"column:updated_at"`
}

func (OAuthIdentity) TableName() string { return "oauth_identities" }

// RefreshToken 是存储的 refresh token；只保存其 SHA-256 哈希
type RefreshToken struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	UserID     int64     `gorm:"column:user_id"`
	TokenHash  string    `gorm:"column:token_hash"`
	ExpiresAt  time.Time `gorm:"column:expires_at"`
	Revoked    bool      `gorm:"column:revoked"`
	ReplacedBy string    `gorm:"column:replaced_by"` // 轮转后后继 token 的哈希
	ClientIP   string    `gorm:"column:client_ip"`
	UserAgent  string    `gorm:"column:user_agent"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }

// Source 是 Alertmanager 接入端点（sources 表）
type Source struct {
	ID          int64      `gorm:"primaryKey;autoIncrement"`
	Name        string     `gorm:"column:name"`
	Token       string     `gorm:"column:token"`
	Description string     `gorm:"column:description"`
	Enabled     bool       `gorm:"column:enabled"`
	LastAlertAt *time.Time `gorm:"column:last_alert_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (Source) TableName() string { return "sources" }

// Template 是消息模板（templates 表）
type Template struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"column:name"`
	ChannelType string    `gorm:"column:channel_type"`
	Content     string    `gorm:"column:content"`
	IsBuiltin   bool      `gorm:"column:is_builtin"`
	Remark      string    `gorm:"column:remark"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (Template) TableName() string { return "templates" }

// Channel 是通知渠道，例如飞书机器人（channels 表）
type Channel struct {
	ID         int64            `gorm:"primaryKey;autoIncrement"`
	Name       string           `gorm:"column:name"`
	Type       string           `gorm:"column:type"`        // feishu / dingtalk / wecom
	WebhookURL string           `gorm:"column:webhook_url"` // 明文存储
	Secret     string           `gorm:"column:secret"`      // 明文存储；空 = 不启用签名校验
	Keyword    string           `gorm:"column:keyword"`
	TemplateID *int64           `gorm:"column:template_id"`
	AtAll      bool             `gorm:"column:at_all"`
	Extra      *json.RawMessage `gorm:"column:extra;type:jsonb"`
	Enabled    bool             `gorm:"column:enabled"`
	CreatedAt  time.Time        `gorm:"column:created_at"`
	UpdatedAt  time.Time        `gorm:"column:updated_at"`
}

func (Channel) TableName() string { return "channels" }

// RoutingRule 将来自某个 source 的告警映射到 channel（routing_rules 表）
type RoutingRule struct {
	ID               int64           `gorm:"primaryKey;autoIncrement"`
	SourceID         int64           `gorm:"column:source_id"`
	Name             string          `gorm:"column:name"`
	Priority         int             `gorm:"column:priority"`
	MatchLabels      json.RawMessage `gorm:"column:match_labels;type:jsonb"`
	ChannelID        int64           `gorm:"column:channel_id"`
	ContinueMatching bool            `gorm:"column:continue_matching"`
	Enabled          bool            `gorm:"column:enabled"`
	CreatedAt        time.Time       `gorm:"column:created_at"`
	UpdatedAt        time.Time       `gorm:"column:updated_at"`
}

func (RoutingRule) TableName() string { return "routing_rules" }

// Alert 是一条存储的告警（alerts 表）
type Alert struct {
	ID          int64           `gorm:"primaryKey;autoIncrement"`
	SourceID    int64           `gorm:"column:source_id"`
	Fingerprint string          `gorm:"column:fingerprint"`
	ContentHash string          `gorm:"column:content_hash"`
	Status      string          `gorm:"column:status"` // firing / resolved
	Alertname   string          `gorm:"column:alertname"`
	Severity    string          `gorm:"column:severity"`
	Labels      json.RawMessage `gorm:"column:labels;type:jsonb"`
	Annotations json.RawMessage `gorm:"column:annotations;type:jsonb"`
	StartsAt    time.Time       `gorm:"column:starts_at"`
	EndsAt      *time.Time      `gorm:"column:ends_at"`
	RawPayload  json.RawMessage `gorm:"column:raw_payload;type:jsonb"`
	Disposition string          `gorm:"column:disposition"` // delivered / deduped / unmatched
	ReceivedAt  time.Time       `gorm:"column:received_at"`
	CreatedAt   time.Time       `gorm:"column:created_at"`
	UpdatedAt   time.Time       `gorm:"column:updated_at"`
}

func (Alert) TableName() string { return "alerts" }

// Delivery 是一条投递尝试记录（deliveries 表）
type Delivery struct {
	ID              int64     `gorm:"primaryKey;autoIncrement"`
	AlertID         int64     `gorm:"column:alert_id"`
	ChannelID       int64     `gorm:"column:channel_id"`
	ChannelName     string    `gorm:"column:channel_name"`
	RuleID          int64     `gorm:"column:rule_id"`      // 0 = 默认兜底
	TriggerType     string    `gorm:"column:trigger_type"` // auto / manual
	Attempts        int       `gorm:"column:attempts"`
	Status          string    `gorm:"column:status"` // success / failed
	HTTPStatus      int       `gorm:"column:http_status"`
	ResponseCode    int       `gorm:"column:response_code"`
	ResponseMsg     string    `gorm:"column:response_msg"`
	DurationMS      int       `gorm:"column:duration_ms"`
	RenderedPayload *string   `gorm:"column:rendered_payload"`
	SentAt          time.Time `gorm:"column:sent_at"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (Delivery) TableName() string { return "deliveries" }
