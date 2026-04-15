package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// 从环境变量或默认值读取配置
	host := getEnv("DB_HOST", "119.45.13.9")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "ccload")
	password := getEnv("DB_PASSWORD", "WAfhn6YGiJQRtyFD")
	database := getEnv("DB_NAME", "ccload")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true",
		user, password, host, port, database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Ping失败: %v", err)
	}

	fmt.Println("✅ 数据库连接成功\n")

	// 1. 查看表结构
	fmt.Println("=== 表结构 ===")
	rows, err := db.Query(`
		SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_DEFAULT, COLUMN_COMMENT
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_NAME = 'logs' AND TABLE_SCHEMA = ?
		ORDER BY ORDINAL_POSITION`, database)
	if err != nil {
		log.Printf("查询表结构失败: %v", err)
	} else {
		defer rows.Close()
		fmt.Printf("%-20s %-15s %-10s %-20s %s\n", "列名", "类型", "可空", "默认值", "注释")
		fmt.Println(string(make([]byte, 100)))
		for rows.Next() {
			var name, dataType, isNullable, defaultVal, comment string
			if err := rows.Scan(&name, &dataType, &isNullable, &defaultVal, &comment); err == nil {
				fmt.Printf("%-20s %-15s %-10s %-20s %s\n", name, dataType, isNullable, defaultVal, comment)
			}
		}
	}

	// 2. 查看 client_ua 统计
	fmt.Println("\n=== client_ua 统计（Top 20）===")
	rows2, err := db.Query(`
		SELECT client_ua, COUNT(*) as cnt
		FROM logs
		WHERE client_ua IS NOT NULL AND client_ua != ''
		GROUP BY client_ua
		ORDER BY cnt DESC
		LIMIT 20`)
	if err != nil {
		log.Printf("查询 UA 统计失败: %v", err)
	} else {
		defer rows2.Close()
		for rows2.Next() {
			var ua string
			var count int
			if err := rows2.Scan(&ua, &count); err == nil {
				if len(ua) > 60 {
					ua = ua[:60] + "..."
				}
				fmt.Printf("%6d | %s\n", count, ua)
			}
		}
	}

	// 3. 查看空 UA 的数量
	fmt.Println("\n=== UA 为空/缺失的日志 ===")
	var emptyCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM logs
		WHERE client_ua IS NULL OR client_ua = ''`).Scan(&emptyCount)
	if err == nil {
		fmt.Printf("空 UA 日志数量: %d\n", emptyCount)
	}

	// 4. 查看最近的5条日志
	fmt.Println("\n=== 最近5条日志 ===")
	rows3, err := db.Query(`
		SELECT id, time, client_ip, client_ua, model, status_code
		FROM logs
		ORDER BY id DESC
		LIMIT 5`)
	if err != nil {
		log.Printf("查询最近日志失败: %v", err)
	} else {
		defer rows3.Close()
		fmt.Printf("%-8s %-12s %-15s %-40s %-20s %s\n", "ID", "时间", "IP", "UA", "模型", "状态码")
		for rows3.Next() {
			var id int
			var time int64
			var ip, ua, model string
			var status int
			if err := rows3.Scan(&id, &time, &ip, &ua, &model, &status); err == nil {
				displayUA := ua
				if len(displayUA) > 35 {
					displayUA = displayUA[:35] + "..."
				}
				fmt.Printf("%-8d %-12d %-15s %-40s %-20s %d\n", id, time, ip, displayUA, model, status)
			}
		}
	}

	// 5. 查看所有渠道的 UA 覆写配置（包括所有字段）
	fmt.Println("\n=== 所有渠道的 UA 覆写配置（详细）===")
	rows4, err := db.Query(`
		SELECT id, name, ua_rewrite_enabled, ua_override, ua_prefix, ua_suffix, ua_config, updated_at
		FROM channels
		ORDER BY id`)
	if err != nil {
		log.Printf("查询渠道配置失败: %v", err)
	} else {
		defer rows4.Close()
		for rows4.Next() {
			var id int
			var name string
			var uaRewriteEnabled int
			var uaOverride, uaPrefix, uaSuffix, uaConfig sql.NullString
			var updatedAt sql.NullInt64
			if err := rows4.Scan(&id, &name, &uaRewriteEnabled, &uaOverride, &uaPrefix, &uaSuffix, &uaConfig, &updatedAt); err == nil {
				fmt.Printf("渠道 #%d %s:\n", id, name)
				if updatedAt.Valid && updatedAt.Int64 > 0 {
					fmt.Printf("  最后更新: %d\n", updatedAt.Int64)
				}
				fmt.Printf("  UA覆写启用: %v\n", uaRewriteEnabled != 0)
				if uaOverride.Valid && uaOverride.String != "" {
					fmt.Printf("  UA覆盖值: %s\n", uaOverride.String)
				}
				if uaPrefix.Valid && uaPrefix.String != "" {
					fmt.Printf("  UA前缀: %s\n", uaPrefix.String)
				}
				if uaSuffix.Valid && uaSuffix.String != "" {
					fmt.Printf("  UA后缀: %s\n", uaSuffix.String)
				}
				// 显示原始 ua_config 值
				if uaConfig.Valid {
					fmt.Printf("  UA配置原始值: [%s]\n", uaConfig.String)
					if uaConfig.String != "" && uaConfig.String != "null" {
						configStr := uaConfig.String
						if len(configStr) > 300 {
							configStr = configStr[:300] + "..."
						}
						fmt.Printf("  UA配置(截断): %s\n", configStr)
						if contains(configStr, "body_operations") || contains(configStr, "header_operations") {
							fmt.Printf("  ⚠️ 包含重写配置\n")
						}
					} else {
						fmt.Printf("  UA配置: (空或null)\n")
					}
				} else {
					fmt.Printf("  UA配置: (NULL)\n")
				}
				fmt.Println()
			}
		}
	}

	// 6. 检查所有渠道表字段
	fmt.Println("\n=== channels 表所有字段 ===")
	rows5, err := db.Query(`
		SELECT COLUMN_NAME, DATA_TYPE
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_NAME = 'channels' AND TABLE_SCHEMA = ?
		ORDER BY ORDINAL_POSITION`, database)
	if err != nil {
		log.Printf("查询字段失败: %v", err)
	} else {
		defer rows5.Close()
		for rows5.Next() {
			var colName, dataType string
			if err := rows5.Scan(&colName, &dataType); err == nil {
				// 只显示与 header/ua/rewrite 相关的字段
				if contains(colName, "ua") || contains(colName, "header") || contains(colName, "rewrite") || contains(colName, "body") || contains(colName, "override") {
					fmt.Printf("  %-25s %s\n", colName, dataType)
				}
			}
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
