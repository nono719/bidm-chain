#!/usr/bin/env bash
set -euo pipefail

FABRIC_SAMPLES_DIR="${FABRIC_SAMPLES_DIR:-/Users/chenminggang/Documents/trae_projects/fabric-samples}"
TEST_NETWORK_DIR="${FABRIC_SAMPLES_DIR}/test-network"
CHAINCODE_PATH="${CHAINCODE_PATH:-/Users/chenminggang/Documents/trae_projects/iot-cross-domain/fabric/chaincode/anchor-go}"
CHAINCODE_NAME="${CHAINCODE_NAME:-anchorcc}"
CHANNEL_NAME="${CHANNEL_NAME:-mychannel}"
CC_VERSION="${CC_VERSION:-1.0}"
CC_SEQUENCE="${CC_SEQUENCE:-1}"
USE_CA="${USE_CA:-false}"

if [[ ! -d "${TEST_NETWORK_DIR}" ]]; then
  echo "test-network not found: ${TEST_NETWORK_DIR}"
  exit 1
fi

if [[ ! -d "${CHAINCODE_PATH}" ]]; then
  echo "chaincode path not found: ${CHAINCODE_PATH}"
  exit 1
fi

cd "${TEST_NETWORK_DIR}"

required_bins=(peer configtxgen cryptogen discover osnadmin orderer)
missing_bin=0
for b in "${required_bins[@]}"; do
  if [[ ! -x "./bin/${b}" ]]; then
    missing_bin=1
    break
  fi
done

if [[ "${missing_bin}" -eq 1 ]]; then
  if [[ ! -x "./install-fabric.sh" ]]; then
    curl --retry 8 --retry-delay 2 --retry-all-errors -fSL -o install-fabric.sh \
      https://raw.githubusercontent.com/hyperledger/fabric/main/scripts/install-fabric.sh
    chmod +x install-fabric.sh
  fi
  ./install-fabric.sh binary || {
    echo "install-fabric.sh 执行失败，请重试。若网络较慢可多次执行直到 ./bin/peer 出现。"
    exit 1
  }
fi

./network.sh down
if [[ "${USE_CA}" == "true" ]]; then
  ./network.sh up createChannel -ca
else
  ./network.sh up createChannel
fi
./network.sh deployCC \
  -ccn "${CHAINCODE_NAME}" \
  -ccp "${CHAINCODE_PATH}" \
  -ccl go \
  -ccv "${CC_VERSION}" \
  -ccs "${CC_SEQUENCE}" \
  -c "${CHANNEL_NAME}"

CERT_PATH="${TEST_NETWORK_DIR}/organizations/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp/signcerts"
KEY_PATH="${TEST_NETWORK_DIR}/organizations/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp/keystore"
TLS_CERT="${TEST_NETWORK_DIR}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt"

cat <<EOF
Fabric test-network and chaincode deployed.
Use the following env vars before starting backend:

export FABRIC_MSP_ID=Org1MSP
export FABRIC_CERT_PATH=${CERT_PATH}
export FABRIC_KEY_PATH=${KEY_PATH}
export FABRIC_TLS_CERT_PATH=${TLS_CERT}
export FABRIC_PEER_ENDPOINT=localhost:7051
export FABRIC_PEER_HOST_ALIAS=peer0.org1.example.com
export FABRIC_CHANNEL=${CHANNEL_NAME}
export FABRIC_CHAINCODE=${CHAINCODE_NAME}
export FABRIC_ANCHOR_FUNCTION=AnchorRecord
export FABRIC_USE_TLS=true
EOF
