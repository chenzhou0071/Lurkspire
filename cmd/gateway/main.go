// gateway — Lurkspire 网关进程（单进程合一：TCP + 大厅 + 房间内嵌）
// 启动：go run ./cmd/gateway --config config.json（缺省内存模式）
package main

import (
	"flag"
	"log"

	"lurkspire/server/internal/gateway"
	"lurkspire/server/internal/lobby"
	"lurkspire/server/internal/room"
	"lurkspire/server/internal/store"
)

func main() {
	configPath := flag.String("config", "config.json", "JSON 配置文件路径（缺省默认内存模式）")
	flag.Parse()

	log.Println("=== Lurkspire Gateway ===")
	log.Printf("配置文件: %s", *configPath)

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}

	// 存储：MySQL（配置了 DSN）或内存（无配置——开发/测试）
	var st store.Store = store.NewMemStore()
	if cfg.MySQLDSN != "" {
		ms, err := store.OpenMySQL(cfg.MySQLDSN)
		if err != nil {
			log.Fatalf("MySQL 连接失败: %v", err)
		}
		defer ms.Close()
		st = ms
		log.Printf("存储: MySQL ✓ (%s)", cfg.MySQLDSN)
	} else {
		log.Println("存储: 内存模式（未配置 mysql_dsn——重启数据丢失）")
	}

	// 单进程合一：房间 Manager + 账号服务内嵌网关（无限对局）
	m := room.NewManager(room.Config{MaxPlayers: 8, TickHz: 30})
	svc := lobby.NewService(st, cfg.TokenSecret)
	log.Println("账号服务: 就绪（注册/登录/好友/背包）")
	log.Println("房间服务: 就绪（1~8 人 / 30Hz / 无限对局）")

	hub := gateway.NewHub(m, svc)
	srv := gateway.NewServer(cfg.Listen, hub)
	if err := srv.Listen(); err != nil {
		log.Fatalf("监听失败: %v", err)
	}
	log.Printf("=== 服务已启动，监听 %s ===", cfg.Listen)
	srv.Serve()
}
