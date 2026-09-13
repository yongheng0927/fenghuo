package service

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/yongheng0927/fenghuo/internal/model"
)

// GormUserStore 用 GORM/PostgreSQL 实现 UserStore
type GormUserStore struct {
	db *gorm.DB
}

// NewGormUserStore 创建 GormUserStore
func NewGormUserStore(db *gorm.DB) *GormUserStore { return &GormUserStore{db: db} }

func (s *GormUserStore) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := s.db.WithContext(ctx).Where("username = ?", username).Take(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *GormUserStore) FindByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	err := s.db.WithContext(ctx).Take(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *GormUserStore) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&model.User{}).Count(&n).Error
	return n, err
}

func (s *GormUserStore) CountBootstrapAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&model.User{}).
		Where("is_bootstrap").Count(&n).Error
	return n, err
}

func (s *GormUserStore) FindBootstrap(ctx context.Context) (*model.User, error) {
	var u model.User
	err := s.db.WithContext(ctx).Where("is_bootstrap").Take(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *GormUserStore) List(ctx context.Context, offset, limit int) ([]model.User, int64, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&model.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []model.User
	err := s.db.WithContext(ctx).Order("id ASC").Offset(offset).Limit(limit).Find(&users).Error
	return users, total, err
}

func (s *GormUserStore) UpdateRoleAndStatus(ctx context.Context, id int64, role string, enabled bool) error {
	return s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		Select("role", "enabled").
		Updates(map[string]any{"role": role, "enabled": enabled}).Error
}

func (s *GormUserStore) UpdateProfile(ctx context.Context, id int64, name, email, avatarURL string) error {
	return s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		Select("name", "email", "avatar_url").
		Updates(map[string]any{"name": name, "email": email, "avatar_url": avatarURL}).Error
}

func (s *GormUserStore) CountEnabledAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&model.User{}).
		Where("role = ? AND enabled", model.RoleAdmin).Count(&n).Error
	return n, err
}

func (s *GormUserStore) Delete(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).Delete(&model.User{}, id).Error
}

func (s *GormUserStore) Create(ctx context.Context, u *model.User) error {
	return s.db.WithContext(ctx).Create(u).Error
}

func (s *GormUserStore) UpdateLastLogin(ctx context.Context, id int64, t time.Time) error {
	return s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		Update("last_login_at", t).Error
}

// UpdatePassword 更新密码哈希并刷新 password_changed_at：JWTAuth 据此让
// 改密前签发的 access token 立即失效（仅吊销 refresh token 挡不住无状态 JWT）
func (s *GormUserStore) UpdatePassword(ctx context.Context, id int64, hash string) error {
	return s.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		Updates(map[string]any{
			"password_hash":       hash,
			"password_changed_at": time.Now(),
		}).Error
}

// GormRefreshTokenStore 用 GORM/PostgreSQL 实现 RefreshTokenStore
type GormRefreshTokenStore struct {
	db *gorm.DB
}

// NewGormRefreshTokenStore 创建 GormRefreshTokenStore
func NewGormRefreshTokenStore(db *gorm.DB) *GormRefreshTokenStore {
	return &GormRefreshTokenStore{db: db}
}

func (s *GormRefreshTokenStore) Create(ctx context.Context, t *model.RefreshToken) error {
	return s.db.WithContext(ctx).Create(t).Error
}

func (s *GormRefreshTokenStore) FindByHash(ctx context.Context, hash string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	err := s.db.WithContext(ctx).Where("token_hash = ?", hash).Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *GormRefreshTokenStore) Save(ctx context.Context, t *model.RefreshToken) error {
	return s.db.WithContext(ctx).Save(t).Error
}

func (s *GormRefreshTokenStore) RevokeByHash(ctx context.Context, hash string) error {
	return s.db.WithContext(ctx).Model(&model.RefreshToken{}).
		Where("token_hash = ? AND revoked = FALSE", hash).
		Update("revoked", true).Error
}

func (s *GormRefreshTokenStore) RevokeAllForUser(ctx context.Context, userID int64) error {
	return s.db.WithContext(ctx).Model(&model.RefreshToken{}).
		Where("user_id = ? AND revoked = FALSE", userID).
		Update("revoked", true).Error
}

func (s *GormRefreshTokenStore) DeleteAllForUser(ctx context.Context, userID int64) error {
	return s.db.WithContext(ctx).Where("user_id = ?", userID).
		Delete(&model.RefreshToken{}).Error
}

// GormOAuthIdentityStore 用 GORM/PostgreSQL 实现 OAuthIdentityStore
type GormOAuthIdentityStore struct {
	db *gorm.DB
}

// NewGormOAuthIdentityStore 创建 GormOAuthIdentityStore
func NewGormOAuthIdentityStore(db *gorm.DB) *GormOAuthIdentityStore {
	return &GormOAuthIdentityStore{db: db}
}

func (s *GormOAuthIdentityStore) DeleteAllForUser(ctx context.Context, userID int64) error {
	return s.db.WithContext(ctx).Where("user_id = ?", userID).
		Delete(&model.OAuthIdentity{}).Error
}

func (s *GormOAuthIdentityStore) FindByProviderUserID(ctx context.Context, provider, providerUserID string) (*model.OAuthIdentity, error) {
	var o model.OAuthIdentity
	err := s.db.WithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", provider, providerUserID).
		Take(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *GormOAuthIdentityStore) Create(ctx context.Context, o *model.OAuthIdentity) error {
	return s.db.WithContext(ctx).Create(o).Error
}
