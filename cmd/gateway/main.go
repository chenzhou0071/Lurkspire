// gateway — Lurkspire 网关进程（单进程合一：TCP 连接 + 房间内嵌）
// 客户端连接 :7777 → Join（房间名）→ 30Hz 对局（房间层驱动）
package main

import (
	"flag"
	"log"

	"lurkspire/server/internal/gateway"
	"lurkspire/server/internal/room"
)

func main() {
	addr := flag.String("addr", ":7777", "listen address")
	flag.Parse()

	// 单进程合一：房间 Manager 内嵌网关（M3 拆双进程时换成 RPC 客户端）
	m := room.NewManager(room.Config{
		MaxPlayers:  8,
		TickHz:      30,
		SettleScore: 20,   // 先到 20 分结算
		SettleTicks: 9000, // 兜底 5 分钟（30Hz × 300s）
	})
	hub := gateway.NewHub(m)
	srv := gateway.NewServer(*addr, hub)
	if err := srv.Listen(); err != nil {
		log.Fatalf("listen: %v", err)
	}
	srv.Serve()
}
