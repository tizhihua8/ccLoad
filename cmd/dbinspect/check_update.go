package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
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

	// 查询所有非空的 UA 配置
	rows, err := db.Query(`
		SELECT id, name, ua_rewrite_enabled, ua_override, ua_prefix, ua_suffix, ua_config, updated_at
		FROM channels
		WHERE ua_rewrite_enabled = 1 
		   OR (ua_override IS NOT NULL AND ua_override != '')
		   OR (ua_config IS NOT NULL AND ua_config != '' AND ua_config != 'null' AND ua_config != '[]')
		ORDER BY updated_at DESC`)
	if err != nil {
		log.Printf("查询失败: %v", err)
		return
	}
	defer rows.Close()

	fmt.Println("=== 有 UA 配置的渠道 ===")
	found := false
	for rows.Next() {
		var id int
		var name string
		var uaRewriteEnabled int
		var uaOverride, uaPrefix, uaSuffix, uaConfig sql.NullString
		var updatedAt sql.NullTime
		if err := rows.Scan(&id, &name, &uaRewriteEnabled, &uaOverride, &uaPrefix, &uaSuffix, &uaConfig, &updatedAt); err == nil {
			found = true
			fmt.Printf("\n渠道 #%d %s:\n", id, name)
			fmt.Printf("  UA覆写启用: %v\n", uaRewriteEnabled != 0)
			fmt.Printf("  UA覆盖: %v\n", uaOverride.String)
			fmt.Printf("  UA前缀: %v\n", uaPrefix.String)
			fmt.Printf("  UA后缀: %v\n", uaSuffix.String)
			fmt.Printf("  UA配置: %v\n", uaConfig.String)
			if updatedAt.Valid {
				fmt.Printf("  更新时间: %v\n", updatedAt.Time)
			}
		}
	}
	if !found {
		fmt.Println("没有找到任何有 UA 配置的渠道")
		fmt.Println("\n下午的配置可能没有保存成功")
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
