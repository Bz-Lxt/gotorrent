// Peer 节点入口：P2P 监听 + HTTP 控制台。
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/Bz-Lxt/gotorrent/peer"
)

func main() {
	listen := flag.String("listen", ":6881", "P2P 协议监听地址")
	control := flag.String("control", ":9000", "HTTP 控制台监听地址")
	dir := flag.String("dir", "downloads", "数据目录（下载文件与种子存放处）")
	trackerURL := flag.String("tracker", "http://localhost:8080/announce", "默认 Tracker 地址")
	flag.Parse()

	p, err := peer.New(*dir)
	if err != nil {
		log.Fatal(err)
	}
	if err := p.Listen(*listen); err != nil {
		log.Fatal(err)
	}

	cs := peer.NewControlServer(p, *trackerURL)
	fmt.Printf("GoTorrent 节点已启动\n")
	fmt.Printf("  控制台:   http://localhost%s/\n", *control)
	fmt.Printf("  P2P 端口: %s\n", *listen)
	fmt.Printf("  数据目录: %s\n", *dir)
	log.Fatal(http.ListenAndServe(*control, cs.Handler()))
}
