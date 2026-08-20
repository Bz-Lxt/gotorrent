# gotorrent

gotorrent

## 构建 / 运行 / 测试

```bash
go build ./...
go test ./...
go vet ./...
```

Go 语言版本（go.mod）：`1.22`。
容器内完整工具链；依赖在镜像构建阶段下载，启动后应可离线编译。
不要使用项目自带的 Dockerfile 做评测构建，必须 `docker build -f benzhi.Dockerfile`。
