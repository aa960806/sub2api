package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUserPlatformQuotasPurgeUnlimitedMigration 锁定升级清理的数据保护边界：
// 只有三档限额全为 NULL 且三档累计用量分别为 0 的行可以删除。
// 任一档已配置限额（含显式 0）或具有非零用量的行，包括软删历史，都必须保留。
// 各窗口必须独立判零，不能改为相加后判零，否则正负用量相抵的历史行会被误删。
//
// 隔离 PostgreSQL fixture：在事务内建立同名 TEMP 表 ON COMMIT DROP，显式将
// search_path 设置为 pg_temp；表只需要 id、三档 limit、三档 usage、deleted_at。
// 为活跃/软删两种状态分别插入以下记录后，执行 FS 中的真实迁移（不重写谓词）：
//   - 三档 limit 为 NULL，usage 为 0/0/0：删除；
//   - 三档 limit 为 NULL，usage 分别为 1/0/0、0/1/0、0/0/1：全部保留；
//   - 三档 limit 为 NULL，usage 为 1/-1/0：保留，防止相加判零；
//   - 任一档 limit 为 0 或正数，usage 全 0：全部保留。
//
// 验证剩余 id 及非零 usage 未变，再执行一次迁移应删除 0 行，最后 ROLLBACK。
// 此 fixture 仅用于隔离副本；默认单测不连接任何数据库。
func TestUserPlatformQuotasPurgeUnlimitedMigration(t *testing.T) {
	content, err := FS.ReadFile("238_purge_unlimited_user_platform_quotas.sql")
	require.NoError(t, err)

	// 去掉注释行后只允许这一条 DELETE。
	var stmts []string
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		stmts = append(stmts, trimmed)
	}
	sql := strings.Join(stmts, " ")
	require.Equal(t, "DELETE FROM user_platform_quotas WHERE daily_limit_usd IS NULL AND weekly_limit_usd IS NULL AND monthly_limit_usd IS NULL AND COALESCE(daily_usage_usd, 0) = 0 AND COALESCE(weekly_usage_usd, 0) = 0 AND COALESCE(monthly_usage_usd, 0) = 0;", sql)
}
