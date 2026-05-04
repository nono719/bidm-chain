#!/usr/bin/env bash
set -euo pipefail

FABRIC_SAMPLES_DIR="${FABRIC_SAMPLES_DIR:-/Users/chenminggang/Documents/trae_projects/fabric-samples}"
TEST_NETWORK_DIR="${FABRIC_SAMPLES_DIR}/test-network"

if [[ ! -d "${TEST_NETWORK_DIR}" ]]; then
  echo "test-network not found: ${TEST_NETWORK_DIR}"
  exit 1
fi

cd "${TEST_NETWORK_DIR}"
./network.sh down

echo "Fabric test-network 已关闭。"
