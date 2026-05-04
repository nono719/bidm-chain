#!/usr/bin/env bash
set -euo pipefail

BACKEND_HEALTH_URL="${BACKEND_HEALTH_URL:-http://localhost:8080/healthz}"
MAX_RETRY="${MAX_RETRY:-40}"
SLEEP_SECONDS="${SLEEP_SECONDS:-2}"

cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/frontend

if [[ ! -d "node_modules" ]]; then
  echo "node_modules 不存在，正在安装依赖..."
  npm install
fi

echo "等待后端就绪: ${BACKEND_HEALTH_URL}"
ok=0
for i in $(seq 1 "${MAX_RETRY}"); do
  if curl -fsS "${BACKEND_HEALTH_URL}" >/dev/null 2>&1; then
    ok=1
    break
  fi
  sleep "${SLEEP_SECONDS}"
done

if [[ "${ok}" -ne 1 ]]; then
  echo "后端健康检查超时，请先启动后端再重试。"
  exit 1
fi

echo "后端已就绪，启动前端..."
npm run dev -- --host 0.0.0.0
