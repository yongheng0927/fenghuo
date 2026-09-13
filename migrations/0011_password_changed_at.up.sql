-- 0011_password_changed_at：记录密码最近修改时间。此前改密（修改密码 /
-- reset-admin-password）只吊销 refresh token，对已签发的无状态 JWT access
-- token 无影响，旧会话最长还能存活 access_ttl（默认 2h）；JWTAuth 中间件
-- 比较 token 签发时间与该字段，让改密前的 access token 立即失效

ALTER TABLE users ADD COLUMN password_changed_at TIMESTAMPTZ;

-- 存量用户回填为建号时间：登录必然晚于建号，已签发的 token 不受影响，
-- 只有迁移之后发生的改密才会踢下线
UPDATE users SET password_changed_at = created_at WHERE password_hash IS NOT NULL;
