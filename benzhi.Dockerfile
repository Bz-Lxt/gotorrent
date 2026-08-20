# 官方 Go 镜像，自带完整工具链（评测时模型需在容器内改代码、编译、跑测试）
# 不要改成多阶段 + alpine：容器里必须留着 go 命令。
FROM golang:1.22

ENV GOTOOLCHAIN=local
ENV GOPROXY=https://goproxy.cn,direct
ENV TZ=Asia/Shanghai

WORKDIR /app

COPY go.mod ./

COPY . .

# 预编译一次，把编译缓存留在镜像里（不影响源码，模型仍可自由修改）
RUN go build ./...

CMD ["bash"]
