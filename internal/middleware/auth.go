// Package middleware 提供 JWT 认证和角色授权
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	fafjwt "github.com/yongheng0927/fenghuo/internal/pkg/jwt"
	"github.com/yongheng0927/fenghuo/internal/service"
)

// JWTAuth 设置的上下文键
const (
	CtxUserID   = "faf.userID"
	CtxUserRole = "faf.userRole"
)

// JWTAuth 校验 Authorization: Bearer access token，加载用户，并拒绝已禁用
// 的账号（返回 401）（FR-5.2，users.enabled=false -> 401）
func JWTAuth(jwtMgr *fafjwt.Manager, users service.UserStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		tokenStr, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		claims, err := jwtMgr.ParseAccessToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		userID, err := claims.UserID()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token subject"})
			return
		}
		user, err := users.FindByID(c.Request.Context(), userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		if user == nil || !user.Enabled {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "account disabled or deleted"})
			return
		}
		// 吊销 refresh token 只能阻止续期，对已签发的无状态 JWT 无效；
		// 比较 token 签发时间与密码修改时间，让改密（含 reset-admin-password）
		// 前的 access token 立即失效。iat 精度为秒，同一秒内签发的 token 可能
		// 残留存活至过期，可接受
		if user.PasswordChangedAt != nil && claims.IssuedAt != nil &&
			claims.IssuedAt.Time.Before(*user.PasswordChangedAt) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session invalidated by password change"})
			return
		}
		c.Set(CtxUserID, user.ID)
		c.Set(CtxUserRole, user.Role)
		c.Next()
	}
}

// roleRank 定义三种角色的顺序：viewer < editor < admin（FR-5.4）
var roleRank = map[string]int{
	"viewer": 1,
	"editor": 2,
	"admin":  3,
}

// RequireRole 仅当当前用户的角色不低于 minRole（"viewer"、"editor" 或
// "admin"）时才放行请求，必须在 JWTAuth 之后执行
func RequireRole(minRole string) gin.HandlerFunc {
	min := roleRank[minRole]
	return func(c *gin.Context) {
		role, _ := c.Get(CtxUserRole)
		rank, ok := roleRank[role.(string)]
		if !ok || rank < min {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient role"})
			return
		}
		c.Next()
	}
}
