package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/yongheng0927/fenghuo/internal/config"
)

// Version 是 /api/v1/system/info 上报的应用版本号（发布时需与镜像 tag 同步更新）
const Version = "0.1.1"

// SystemHandler 提供健康/就绪探针和系统信息接口
type SystemHandler struct {
	cfg *config.Config
	db  *gorm.DB
}

// NewSystemHandler 创建 SystemHandler
func NewSystemHandler(cfg *config.Config, db *gorm.DB) *SystemHandler {
	return &SystemHandler{cfg: cfg, db: db}
}

// Healthz 处理 GET /healthz（存活探针，不检查任何依赖）
func (h *SystemHandler) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readyz 处理 GET /readyz（就绪探针，会实际 ping 数据库）
func (h *SystemHandler) Readyz(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "database not ready")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		fail(c, http.StatusServiceUnavailable, "database not ready")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

// Info 处理 GET /api/v1/system/info 该接口公开：登录页需要在认证前拿到
// oauth_enabled 来决定是否展示 OAuth 登录入口，返回字段均不敏感
func (h *SystemHandler) Info(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":       Version,
		"root_url":      h.cfg.Server.RootURL,
		"oauth_enabled": h.cfg.OAuth.Enabled,
	})
}
