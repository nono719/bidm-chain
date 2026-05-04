#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
USERNAME="${USERNAME:-admin}"
PASSWORD="${PASSWORD:-123456}"
DEVICE_DID="${DEVICE_DID:-did:iot:device-001}"
FROM_DOMAIN="${FROM_DOMAIN:-domain-a}"
TO_DOMAIN="${TO_DOMAIN:-domain-b}"
DEVICE_NAME="${DEVICE_NAME:-温湿度传感器}"
CREDENTIAL="${CREDENTIAL:-dev-secret-001}"

extract_json() {
  local key="$1"
  python3 -c "import json,sys; print(json.load(sys.stdin)$key)"
}

echo "[1/7] 登录"
login_resp="$(curl -sS -X POST "${BASE_URL}/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"
token="$(echo "${login_resp}" | extract_json "['data']['token']")"
if [[ -z "${token}" || "${token}" == "None" ]]; then
  echo "登录失败: ${login_resp}"
  exit 1
fi

auth_header="Authorization: Bearer ${token}"

echo "[2/7] 创建设备(重复创建会提示已存在)"
curl -sS -X POST "${BASE_URL}/api/devices" \
  -H "${auth_header}" -H 'Content-Type: application/json' \
  -d "{\"deviceDid\":\"${DEVICE_DID}\",\"domainCode\":\"${FROM_DOMAIN}\",\"displayName\":\"${DEVICE_NAME}\",\"credential\":\"${CREDENTIAL}\"}" >/dev/null || true

echo "[3/7] 预言机状态上报"
state="RISKY"
for i in $(seq 1 30); do
  oracle_resp="$(curl -sS -X POST "${BASE_URL}/api/oracle/report/${DEVICE_DID}" -H "${auth_header}")"
  state="$(echo "${oracle_resp}" | python3 -c "import json,sys; print(json.load(sys.stdin).get('data',{}).get('state','RISKY'))")"
  if [[ "${state}" == "TRUSTED" ]]; then
    break
  fi
done
if [[ "${state}" != "TRUSTED" ]]; then
  echo "设备状态多次上报后仍非 TRUSTED，停止联调"
  exit 1
fi

echo "[4/7] 发起跨域认证"
cross_req_resp="$(curl -sS -X POST "${BASE_URL}/api/cross/request" \
  -H "${auth_header}" -H 'Content-Type: application/json' \
  -d "{\"deviceDid\":\"${DEVICE_DID}\",\"toDomainCode\":\"${TO_DOMAIN}\"}")"
request_id="$(echo "${cross_req_resp}" | extract_json "['data']['requestId']")"
challenge="$(echo "${cross_req_resp}" | extract_json "['data']['challenge']")"
if [[ -z "${request_id}" || "${request_id}" == "None" || -z "${challenge}" || "${challenge}" == "None" ]]; then
  echo "发起跨域认证失败: ${cross_req_resp}"
  exit 1
fi

echo "[5/7] 计算签名并提交验签"
signature="$(printf '%s' "${challenge}:${CREDENTIAL}" | shasum -a 256 | awk '{print $1}')"
verify_resp="$(curl -sS -X POST "${BASE_URL}/api/cross/verify" \
  -H "${auth_header}" -H 'Content-Type: application/json' \
  -d "{\"requestId\":\"${request_id}\",\"signature\":\"${signature}\"}")"
verify_status="$(echo "${verify_resp}" | extract_json "['code']")"
if [[ "${verify_status}" != "0" ]]; then
  echo "跨域验签失败: ${verify_resp}"
  exit 1
fi

echo "[6/7] 执行受保护操作"
op_resp="$(curl -sS -X POST "${BASE_URL}/api/operations/protected" \
  -H "${auth_header}" \
  -H "X-Device-DID: ${DEVICE_DID}" \
  -H "X-Target-Domain: ${TO_DOMAIN}" \
  -H 'Content-Type: application/json' \
  -d "{\"deviceDid\":\"${DEVICE_DID}\",\"domainCode\":\"${TO_DOMAIN}\",\"operation\":\"READ_REMOTE_PROFILE\",\"payload\":\"{\\\"resource\\\":\\\"profile\\\"}\"}")"
op_status="$(echo "${op_resp}" | extract_json "['code']")"
if [[ "${op_status}" != "0" ]]; then
  echo "受保护操作失败: ${op_resp}"
  exit 1
fi

echo "[7/7] 成功，关键结果如下"
tx_hash="$(echo "${verify_resp}" | extract_json "['data']['txHash']")"
echo "requestId=${request_id}"
echo "txHash=${tx_hash}"
echo "操作已放行，跨域认证门禁验证通过。"
