// Tracker 服务器入口。
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Bz-Lxt/gotorrent/tracker"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP 监听地址")
	interval := flag.Int("interval", 15, "建议 Peer 汇报间隔（秒）")
	peerTTL := flag.Duration("peer-ttl", 90*time.Second, "节点超时剔除时间")
	maxPeers := flag.Int("max-peers", 50, "每次 announce 最多返回的节点数")
	flag.Parse()

	tr := tracker.New(*peerTTL)
	srv := tracker.NewServer(tr, *interval, *maxPeers)

	fmt.Printf("GoTorrent Tracker 已启动\n")
	fmt.Printf("  管理页面:  http://localhost%s/\n", *addr)
	fmt.Printf("  Announce:  http://localhost%s/announce\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, srv.Handler()))
}
