// reset-admin-password 子命令：在容器/服务器内直接执行，重置初始管理员密码
// 适用于管理员密码丢失的应急场景（FR-5.1 的运维兜底）
//
// 用法：
//
//	fenghuo reset-admin-password                  # 生成随机密码并打印
//	fenghuo reset-admin-password --password NEW   # 指定新密码
//
// 配置读取与服务模式一致（环境变量 > config.yaml），因此容器内执行时
// 天然使用与运行中实例相同的数据库；初始管理员通过 users.is_bootstrap 定位
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"math/big"
	"os"

	"github.com/yongheng0927/fenghuo/internal/config"
	"github.com/yongheng0927/fenghuo/internal/database"
	"github.com/yongheng0927/fenghuo/internal/service"
)

func resetAdminPassword(args []string) error {
	fs := flag.NewFlagSet("reset-admin-password", flag.ContinueOnError)
	pw := fs.String("password", "", "new password; a random one is generated if empty")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// flag 包遇到第一个非 flag 参数即停止解析，位置参数会被静默忽略
	//（"reset-admin-password MyPass" 不会报错，而是生成随机密码），显式拒绝
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q: use --password to specify the new password, or omit it to generate a random one", fs.Arg(0))
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.Open(cfg.Database.DSN())
	if err != nil {
		return err
	}
	users := service.NewGormUserStore(db)
	tokens := service.NewGormRefreshTokenStore(db)
	oauth := service.NewGormOAuthIdentityStore(db)
	svc := service.NewUserAdminService(users, tokens, oauth)

	ctx := context.Background()
	admin, err := users.FindBootstrap(ctx)
	if err != nil {
		return err
	}
	if admin == nil {
		return fmt.Errorf("setup not completed yet: no bootstrap admin in database")
	}

	newPw := *pw
	generated := false
	if newPw == "" {
		newPw, err = randomPassword(16)
		if err != nil {
			return err
		}
		generated = true
	}
	// 初始管理员重置自己的密码，天然通过 ResetPassword 的权限校验；
	// 同时吊销其全部会话，防止旧会话继续可用
	if err := svc.ResetPassword(ctx, admin.ID, admin.ID, newPw); err != nil {
		return err
	}
	if generated {
		fmt.Printf("password of %q has been reset to: %s\n", admin.Username, newPw)
	} else {
		fmt.Printf("password of %q has been reset\n", admin.Username)
	}
	fmt.Fprintln(os.Stderr, "all existing sessions of this account were revoked")
	return nil
}

// randomPassword 生成 n 位字母数字随机密码
func randomPassword(n int) (string, error) {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := range buf {
		k, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = alphabet[k.Int64()]
	}
	return string(buf), nil
}
