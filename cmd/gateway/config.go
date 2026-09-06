// config.go — 启动配置：JSON 文件（config.json.example 为模板；源文件不追踪）
package main

import (
	"encoding/json"
	"os"
)

// Config 网关配置
type Config struct {
	Listen      string `json:"listen"`       // TCP 监听地址
	MySQLDSN    string `json:"mysql_dsn"`    // 空 = 内存存储（无 MySQL 也能跑）
	TokenSecret string `json:"token_secret"` // Token HMAC 密钥
}

// DefaultConfig 无配置文件时的默认值（内存模式——开发/测试）
func DefaultConfig() Config {
	return Config{
		Listen:      ":7777",
		MySQLDSN:    "",
		TokenSecret: "dev-secret-change-me",
	}
}

// LoadConfig 读 JSON 配置（缺失 = 默认；字段缺省 = 默认）
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // 无配置文件：默认（内存）
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
