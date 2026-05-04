#!/usr/bin/env bash
set -euo pipefail

TEST_NETWORK_DIR="${TEST_NETWORK_DIR:-/Users/chenminggang/claude/trae_projects/fabric-samples/test-network}"
CHAINCODE_NAME="${CHAINCODE_NAME:-anchorcc}"
CHANNEL_NAME="${CHANNEL_NAME:-mychannel}"
MYSQL_CONTAINER="${MYSQL_CONTAINER:-bidm-mysql}"
MYSQL_DB_NAME="${MYSQL_DB_NAME:-iot_auth}"

export FABRIC_MSP_ID=Org1MSP
export FABRIC_CERT_PATH="${TEST_NETWORK_DIR}/organizations/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp/signcerts"
export FABRIC_KEY_PATH="${TEST_NETWORK_DIR}/organizations/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp/keystore"
export FABRIC_TLS_CERT_PATH="${TEST_NETWORK_DIR}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt"
export FABRIC_PEER_ENDPOINT=localhost:7051
export FABRIC_PEER_HOST_ALIAS=peer0.org1.example.com
export FABRIC_CHANNEL="${CHANNEL_NAME}"
export FABRIC_CHAINCODE="${CHAINCODE_NAME}"
export FABRIC_ANCHOR_FUNCTION=AnchorRecord
export FABRIC_USE_TLS=true

# If user already provides MYSQL_DSN, keep it unchanged.
if [[ -z "${MYSQL_DSN:-}" ]]; then
  if docker ps --format '{{.Names}}' | grep -q "^${MYSQL_CONTAINER}$"; then
    mysql_port="$(docker port "${MYSQL_CONTAINER}" 3306/tcp | awk -F: '{print $2}')"
    mysql_root_pwd="$(docker inspect "${MYSQL_CONTAINER}" --format '{{range .Config.Env}}{{println .}}{{end}}' | awk -F= '/^MYSQL_ROOT_PASSWORD=/{print $2}' | tail -n1)"
    if [[ -n "${mysql_port}" && -n "${mysql_root_pwd}" ]]; then
      docker exec "${MYSQL_CONTAINER}" mysql -uroot -p"${mysql_root_pwd}" -e "CREATE DATABASE IF NOT EXISTS ${MYSQL_DB_NAME} CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;" >/dev/null 2>&1 || true
      export MYSQL_DSN="root:${mysql_root_pwd}@tcp(127.0.0.1:${mysql_port})/${MYSQL_DB_NAME}?charset=utf8mb4&parseTime=True&loc=Local"
      echo "Auto MYSQL_DSN detected from container ${MYSQL_CONTAINER}: 127.0.0.1:${mysql_port}/${MYSQL_DB_NAME}"
    fi
  fi
fi

# Final fallback for local mysql defaults.
if [[ -z "${MYSQL_DSN:-}" ]]; then
  export MYSQL_DSN="root:root@tcp(127.0.0.1:3306)/${MYSQL_DB_NAME}?charset=utf8mb4&parseTime=True&loc=Local"
  echo "Fallback MYSQL_DSN in use: 127.0.0.1:3306/${MYSQL_DB_NAME}"
fi

cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/backend
go run ./cmd/server
