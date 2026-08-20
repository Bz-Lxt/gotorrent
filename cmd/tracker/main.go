// Tracker 服务器入口。
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Bz-Lxt/gotorrent/tracker"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP 监听地址")
	interval := flag.Int("interval", 15, "建议 Peer 汇报间隔（秒）")
	peerTTL := flag.Duration("peer-ttl", 90*time.Second, "节点超时剔除时间")
	maxPeers := flag.Int("max-peers", 50, "每次 announce 最多返回的节点数")
	flag.Parse()

	dataDir := os.Getenv("DATA_DIR")
	tr, err := tracker.Open(*peerTTL, dataDir)
	if err != nil {
		log.Fatalf("打开 Tracker 失败: %v", err)
	}
	srv := tracker.NewServer(tr, *interval, *maxPeers)

	fmt.Printf("GoTorrent Tracker 已启动\n")
	if dataDir != "" {
		fmt.Printf("  数据目录:  %s\n", dataDir)
	}
	fmt.Printf("  管理页面:  http://localhost%s/\n", *addr)
	fmt.Printf("  Announce:  http://localhost%s/announce\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, srv.Handler()))
}
