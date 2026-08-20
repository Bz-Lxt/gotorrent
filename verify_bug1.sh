#!/usr/bin/env bash
# 复现 gotorrent-1 的缺陷：Tracker 在处理 announce 时会卡死不返回。
#
# 退出码：
#   0 = 缺陷未复现（announce 正常返回 200）
#   1 = 缺陷复现（announce 请求挂起 / 超时）
#   2 = 环境 / 构建 / 启动失败
set -uo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT/docker-compose.yml}"
# 并发脚本互不抢同一 compose 项目；不要写死 --platform，跟宿主 arch 走。
PROJECT="gogo-verify-1-$$"
PORT="${PORT:-18133}"
# announce 请求最长等待时间（秒）。正常返回在毫秒级；挂起会触到该上限。
ANNOUNCE_TIMEOUT="${ANNOUNCE_TIMEOUT:-8}"

cleanup() {
  docker compose -p "$PROJECT" -f "$COMPOSE_FILE" down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

# 段1：用仓库自带 compose 起服务。构建/启动失败必须 exit 2，不能当成 bug 复现。
# 镜像是官方多架构 golang 镜像，不 pin --platform。输出重定向以保持判读清爽。
if ! docker compose -p "$PROJECT" -f "$COMPOSE_FILE" up -d --wait >/tmp/gotorrent_up.log 2>&1; then
  echo "[EXPECT] docker compose up succeeded (容器健康探针 200)"
  echo "[ACTUAL] compose up 失败，服务未能启动或未通过健康探针"
  tail -n 20 /tmp/gotorrent_up.log 2>/dev/null
  exit 2
fi

# 健康探针必须 200，否则视为环境失败。
health_code=$(curl -s -o /dev/null -w '%{http_code}' -m 5 "http://127.0.0.1:${PORT}/health" 2>/dev/null)
if [ "$health_code" != "200" ]; then
  echo "[EXPECT] GET /health -> 200"
  echo "[ACTUAL] GET /health -> ${health_code}"
  exit 2
fi

# 段2：走容器对外入口复现 announce。
# 构造一个合法的 announce 请求（40 位十六进制 info_hash、peer_id、port）。
ANN_URL="http://127.0.0.1:${PORT}/announce?info_hash=dddddddddddddddddddddddddddddddddddddddd&peer_id=-GT0001-verify0001&port=6881&event=started&name=verify.bin&length=1024&left=0"

start=$(date +%s)
ann_code=$(curl -s -o /tmp/gotorrent_ann.out -w '%{http_code}' -m "$ANNOUNCE_TIMEOUT" "$ANN_URL" 2>/dev/null)
end=$(date +%s)
elapsed=$((end - start))

# 段3：对外部可观测量断言。
echo "[EXPECT] GET /announce 在数秒内返回 HTTP 200（正常应在毫秒级返回）"
echo "[ACTUAL] GET /announce -> http_code=${ann_code}, 耗时 ${elapsed}s, body=$(head -c 120 /tmp/gotorrent_ann.out 2>/dev/null)"

if [ "$ann_code" = "000" ]; then
  # 请求未在超时内返回 -> 服务把 announce 调用挂住 -> 缺陷复现。
  exit 1
fi

if [ "$ann_code" = "200" ]; then
  # 正常返回 -> 缺陷不存在（已修复或 main 干净）。
  exit 0
fi

# 其他非 200 也视为缺陷表现，记为复现。
exit 1
