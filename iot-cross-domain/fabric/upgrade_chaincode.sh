#!/usr/bin/env bash
set -euo pipefail

FABRIC_SAMPLES_DIR="${FABRIC_SAMPLES_DIR:-/Users/chenminggang/Documents/trae_projects/fabric-samples}"
TEST_NETWORK_DIR="${FABRIC_SAMPLES_DIR}/test-network"
CHAINCODE_PATH="${CHAINCODE_PATH:-/Users/chenminggang/Documents/trae_projects/iot-cross-domain/fabric/chaincode/anchor-go}"
CHAINCODE_NAME="${CHAINCODE_NAME:-anchorcc}"
CHANNEL_NAME="${CHANNEL_NAME:-mychannel}"
CC_VERSION="${CC_VERSION:-1.1}"
CC_SEQUENCE="${CC_SEQUENCE:-2}"

if [[ ! -d "${TEST_NETWORK_DIR}" ]]; then
  echo "test-network not found: ${TEST_NETWORK_DIR}"
  exit 1
fi

if [[ ! -d "${CHAINCODE_PATH}" ]]; then
  echo "chaincode path not found: ${CHAINCODE_PATH}"
  exit 1
fi

cd "${TEST_NETWORK_DIR}"
./network.sh deployCC \
  -ccn "${CHAINCODE_NAME}" \
  -ccp "${CHAINCODE_PATH}" \
  -ccl go \
  -ccv "${CC_VERSION}" \
  -ccs "${CC_SEQUENCE}" \
  -c "${CHANNEL_NAME}"

echo "链码升级完成: ${CHAINCODE_NAME} v${CC_VERSION} seq=${CC_SEQUENCE}"
