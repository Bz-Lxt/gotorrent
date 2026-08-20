# gotorrent

Tracker 协调分片持有者，Peer 侧另有入口；本仓 0-1 以 Tracker HTTP 为准。

时钟默认 Asia/Shanghai。数据目录由 `DATA_DIR` 指定，重启后从快照或 WAL 恢复。

```bash
docker compose up -d --wait
curl -sf http://127.0.0.1:18133/health
```

不要把本仓做成桌面工具或业务中台。
