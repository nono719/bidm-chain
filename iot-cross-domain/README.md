# BIDM-Chain 物联网身份管理与跨域认证系统

基于 Hyperledger Fabric（联盟链）+ Go + Vue3 的物联网设备身份与跨域认证演示系统。  
系统包含：设备 DID 注册/解析、跨域认证审批、预言机状态校验、受保护操作门禁、信任策略、审计与监控。

## 1. 系统架构

- 前端：`frontend`（Vue3 + Vite + Ant Design Vue）
- 后端：`backend`（Gin + GORM + JWT）
- 数据库：MySQL（业务数据）
- 联盟链：Fabric test-network（链上锚定 `anchorcc/AnchorRecord`）
- 预言机：节点管理 + 门限配置 + 状态上报 + 跨域前置校验

## 2. 环境要求

- macOS / Linux（推荐）
- Docker >= 24（用于 Fabric 与 MySQL）
- Go >= 1.25
- Node.js >= 20，npm >= 10
- `curl`、`bash`、`python3`（脚本联调用）

## 3. 目录说明

- `backend/cmd/server/main.go`：后端入口与路由
- `backend/internal/api/handlers.go`：核心业务接口
- `backend/internal/middleware/cross_domain_gate.go`：跨域门禁
- `fabric/setup_test_network.sh`：Fabric 一键部署
- `backend/run_with_fabric_test_network.sh`：带 Fabric 环境启动后端
- `frontend/start_frontend.sh`：等待后端健康后启动前端

## 4. 快速部署（推荐）

### 4.1 启动 Fabric 联盟链与链码

```bash
cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/fabric
bash setup_test_network.sh
```

### 4.2 准备 MySQL（Docker 示例）

```bash
docker run -d --name bidm-mysql \
  -e MYSQL_ROOT_PASSWORD=root \
  -e MYSQL_DATABASE=iot_auth \
  -p 3307:3306 mysql:8.0
```

> 注意：宿主端口建议 **3307**（避免与本机已有的 3306 冲突）。`run_with_fabric_test_network.sh` 会自动通过 `docker inspect` 探测容器实际映射端口生成 `MYSQL_DSN`，无需手动改。

### 4.3 启动后端

```bash
cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/backend
bash run_with_fabric_test_network.sh
```

说明：脚本会自动注入 Fabric 环境变量，并尝试自动识别 `bidm-mysql` 容器生成 `MYSQL_DSN`。

### 4.4 启动前端

```bash
cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/frontend
bash start_frontend.sh
```

默认访问：

- 前端：`http://localhost:5173`
- 后端健康检查：`http://localhost:8080/healthz`
- 默认管理员：`admin / 123456`

### 4.5 服务状态检查

任何时候排查"系统起没起来"，按这一个顺序检查四件事：

```bash
# 1. 联盟链 + MySQL 容器（应该看到 4 个 Up）
docker ps --format '{{.Names}}\t{{.Status}}' | grep -E "peer0|orderer|mysql"

# 2. 后端
curl http://localhost:8080/healthz
# 期待: {"status":"ok"}

# 3. 前端
curl -o /dev/null -w "HTTP %{http_code}\n" http://localhost:5173
# 期待: HTTP 200

# 4. 端口占用情况
lsof -i :8080 -i :5173 | grep LISTEN
```

四个容器名（端口）：

| 容器 | 用途 | 端口 |
|---|---|---|
| `peer0.org1.example.com` | Fabric Org1 Peer | 7051 / 9444 |
| `peer0.org2.example.com` | Fabric Org2 Peer | 9051 / 9445 |
| `orderer.example.com` | Fabric Raft 排序节点 | 7050 |
| `bidm-mysql` | MySQL 业务库 | 3307→3306 |

### 4.6 后台启动（演示推荐）

前台运行（4.3、4.4）适合开发，但终端被占用。**演示和长时间运行**建议用 `nohup` 后台启动，日志写到 `/tmp`：

```bash
# 后端 → 后台
cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/backend
nohup bash run_with_fabric_test_network.sh > /tmp/iot-backend.log 2>&1 &
disown
until curl -fsS http://localhost:8080/healthz >/dev/null 2>&1; do sleep 1; done
echo "✓ 后端 ready"

# 前端 → 后台
cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/frontend
nohup bash start_frontend.sh > /tmp/iot-frontend.log 2>&1 &
disown
until curl -fsS http://localhost:5173 >/dev/null 2>&1; do sleep 1; done
echo "✓ 前端 ready"
```

### 4.7 一键重启（前后端全部）

```bash
# 杀掉旧进程
pkill -f "go run ./cmd/server"; pkill -f "/exe/server"; pkill -f "vite"
sleep 2

# 后端
cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/backend
nohup bash run_with_fabric_test_network.sh > /tmp/iot-backend.log 2>&1 & disown
until curl -fsS http://localhost:8080/healthz >/dev/null 2>&1; do sleep 1; done

# 前端
cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/frontend
nohup bash start_frontend.sh > /tmp/iot-frontend.log 2>&1 & disown
until curl -fsS http://localhost:5173 >/dev/null 2>&1; do sleep 1; done

echo "✓ All services up: http://localhost:5173"
```

### 4.8 停止服务

```bash
# 仅停应用层（保留 Fabric / MySQL 容器，下次启动最快）
pkill -f "go run ./cmd/server"; pkill -f "/exe/server"
pkill -f "vite"
# 兜底（按端口杀进程）
kill $(lsof -ti :8080) 2>/dev/null
kill $(lsof -ti :5173) 2>/dev/null

# 完整停（包括容器，下次启动需要重新等链 ready）
docker stop orderer.example.com peer0.org1.example.com peer0.org2.example.com bidm-mysql
```

### 4.9 日志查看

```bash
# 后端日志（看 Gin 路由 / Fabric 调用 / 链上锚定）
tail -f /tmp/iot-backend.log

# 前端日志（Vite 编译输出 / HMR）
tail -f /tmp/iot-frontend.log

# 容器日志
docker logs --tail 50 orderer.example.com
docker logs --tail 50 peer0.org1.example.com
docker logs --tail 50 bidm-mysql
```

### 4.10 故障恢复速查

| 现象 | 一句话排查 |
|---|---|
| 后端起来但 anchor 失败 `failed to collect enough transaction endorsements` | 两个 chaincode 容器（`dev-peer0.org1.example.com-anchorcc_*`）是否 running，`docker start <name>` 即可 |
| 后端起来但 anchor 失败 `no orderers could successfully process transaction` | Orderer 容器没起 / 刚起没就绪，等 ~12s 再试 |
| 前端能开但 API 401 | sessionStorage 里 token 过期，重新登录 |
| 跨域门禁 403 `cross-domain authentication expired` | 数据库里 session.expires_at < 当前时间，重新发起跨域认证 |
| 跨域门禁 400 `missing X-Device-DID` | 前端 fetch 调用没带 header（一般是登录态没恢复，刷新页面） |

## 5. 手动环境变量配置指南（后端）

如不使用 `run_with_fabric_test_network.sh`，需手动配置以下变量：

### 5.1 基础配置

```bash
export APP_PORT=8080
export MYSQL_DSN='root:root@tcp(127.0.0.1:3306)/iot_auth?charset=utf8mb4&parseTime=True&loc=Local'
export JWT_SECRET='change-this-secret'
```

### 5.2 Fabric 网关配置

```bash
export FABRIC_MSP_ID=Org1MSP
export FABRIC_CERT_PATH='/path/to/User1@org1.example.com/msp/signcerts'
export FABRIC_KEY_PATH='/path/to/User1@org1.example.com/msp/keystore'
export FABRIC_TLS_CERT_PATH='/path/to/peer0.org1.example.com/tls/ca.crt'
export FABRIC_PEER_ENDPOINT='localhost:7051'
export FABRIC_PEER_HOST_ALIAS='peer0.org1.example.com'
export FABRIC_CHANNEL='mychannel'
export FABRIC_CHAINCODE='anchorcc'
export FABRIC_ANCHOR_FUNCTION='AnchorRecord'
export FABRIC_USE_TLS='true'
```

然后运行：

```bash
cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/backend
go run ./cmd/server
```

## 6. 预言机节点公钥配置（符合实际）

系统管理页面要求输入真实 PEM 公钥（PKIX）：

```text
-----BEGIN PUBLIC KEY-----
...
-----END PUBLIC KEY-----
```

后端会严格校验：

- 必须是 PEM 格式
- `Type` 必须为 `PUBLIC KEY`
- 必须能被 `x509.ParsePKIXPublicKey` 解析
- 算法支持：`RSA / ECDSA / ED25519`

### 6.1 推荐生成方式（OpenSSL）

```bash
# 生成私钥（ECDSA P-256）
openssl ecparam -name prime256v1 -genkey -noout -out oracle-node.key

# 导出公钥（PEM）
openssl ec -in oracle-node.key -pubout -out oracle-node.pub.pem
```

将 `oracle-node.pub.pem` 内容粘贴到“系统管理 > 预言机节点注册/轮换”。

## 7. 演示步骤（含双账号同步）

### 7.1 场景目标

- 页面A：默认管理员（`admin`）
- 页面B：域管理员（如 `domain-b-admin`）
- 域管理员发起跨域认证后，管理员审批通过，域管理员页面自动同步状态到 `VERIFIED`

### 7.2 操作流程

1. 在两个独立标签页分别打开系统（建议直接新开标签，不用复制标签）。
2. 页面A登录 `admin`；页面B登录域管理员账号。
3. 页面B进入“跨域认证”，选择设备并发起请求、提交验签。
4. 页面A进入“跨域审批”，对该请求执行“通过”。
5. 页面B保持在“跨域认证”页，无需手动刷新，状态会自动刷新并展示：
   - 审批状态 `VERIFIED`
   - AuthToken 信息
   - 链上回执（交易哈希、区块高度）
6. 页面B在“受保护操作”卡片选择操作并执行，验证跨域门禁放行。

## 8. 一键联调脚本（可选）

后端提供端到端脚本：

```bash
cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/backend
bash e2e_flow.sh
```

脚本会自动执行：登录 -> 创建设备 -> 预言机上报 -> 跨域申请/验签 -> 受保护操作。

## 9. 常见问题排查

- `listen tcp :8080: bind: address already in use`
  - 说明端口被占用，释放占用进程后重启后端。
- 前端提示后端不可达
  - 检查 `http://localhost:8080/healthz` 是否返回 `{"status":"ok"}`。
- 跨域认证被拒绝（oracle）
  - 检查活跃预言机节点数与门限；
  - 重新触发设备状态上报；
  - 确认设备状态为可信且未过期。
- 节点公钥注册失败
  - 确认是 `BEGIN/END PUBLIC KEY` 的 PEM 公钥，不是私钥或证书。

## 10. 查看链上数据（开发 / 答辩 / 排查用）

链上数据分两种：① 区块数据（block 元信息 + 交易列表） ② 链码状态（anchorcc 在账本里存的 key-value）。下面给三条由浅到深的查询路径。

### 10.1 通过前端 UI（最直观）

打开「联盟链浏览」`/chain`：

| 区域 | 显示什么 |
|---|---|
| 顶部 KPI 卡 | 区块高度（链上实时）、累计上链笔数、节点在线数、最近上链时间 + TxHash 短码 |
| 联盟拓扑图 | Channel / Org / Peer / Orderer / Chaincode 关系 |
| 区块哈希链 | 最近 N 块的 DataHash + PrevHash，可直观看到前向哈希指针 |
| 区块列表 | 倒序展示每块的业务锚定记录 |
| 点「查看」打开区块详情 modal | DataHash / PrevHash / TxCount + 该块所有 anchor 的**完整** digest / TxHash（点击复制） |
| 链上读验证（右下） | 输入 TxHash 或 BizRef → 后端**实时调 qscc** 取链上记录 + **anchorcc.QueryAnchor** 读 digest，与数据库副本对比一致性 |
| 故障容错演示卡（管理员） | 「发起一次测试上链」→ 实时返回 TxHash + 区块高度 + Org1+Org2 双方背书 |

### 10.2 通过后端 REST API（curl / Postman）

先拿 JWT：

```bash
TOKEN=$(curl -fsS -X POST -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"123456"}' \
  http://localhost:8080/api/auth/login \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])")
H="Authorization: Bearer $TOKEN"
```

**① 当前链整体状态**（来自 qscc.GetChainInfo）：

```bash
curl -sS -H "$H" http://localhost:8080/api/chain/info | python3 -m json.tool
```
返回 `height` / `currentBlockHash` / `previousBlockHash` / `totalAnchors`。

**② 单个区块的元数据 + 该块内业务锚定**：

```bash
# 查区块 #5
curl -sS -H "$H" http://localhost:8080/api/chain/blocks/5 | python3 -m json.tool
```

**③ 用 TxHash 真正去链上验证一条交易**：

```bash
curl -sS -H "$H" -H "Content-Type: application/json" -X POST \
  -d '{"txHash":"<完整 64 位 hex>"}' \
  http://localhost:8080/api/chain/verify | python3 -m json.tool
```
返回中：
- `chainTx.validationMessage: "VALID"` — Fabric 验证码
- `chainTx.endorsers[]` — Org1MSP/peer0.org1 + Org2MSP/peer0.org2 真实证书 + 签名
- `digestOnChain` — anchorcc.QueryAnchor 读出的链上 digest
- `digestInDB vs digestOnChain` — 数据库副本与链上记录对比

**④ 最近 N 块的哈希链**：

```bash
curl -sS -H "$H" "http://localhost:8080/api/chain/hashchain?limit=10" | python3 -m json.tool
```

**⑤ 拓扑图数据 + 节点在线探测**：

```bash
curl -sS -H "$H" http://localhost:8080/api/chain/topology | python3 -m json.tool
```

### 10.3 通过 Fabric peer CLI 原生查（绕过后端，证明数据真在链上）

**这是答辩时最有说服力的方式** —— 直接进 docker 容器跑 Fabric 自带的 peer 命令，不经过任何业务代码。

**① 查整个链的状态**：

```bash
docker exec \
  -e CORE_PEER_LOCALMSPID=Org1MSP \
  -e CORE_PEER_MSPCONFIGPATH=/etc/hyperledger/fabric/msp \
  -e CORE_PEER_TLS_ENABLED=true \
  -e CORE_PEER_TLS_ROOTCERT_FILE=/etc/hyperledger/fabric/tls/ca.crt \
  peer0.org1.example.com peer channel getinfo -c mychannel
```
输出：`Blockchain info: {"height":N,"currentBlockHash":"...","previousBlockHash":"..."}`

**② 查 anchorcc 链码中某条业务记录**（按业务键直接读）：

```bash
docker exec peer0.org1.example.com peer chaincode query \
  -C mychannel -n anchorcc \
  -c '{"Args":["QueryAnchor","cross_auth","REQ-1777884842-3352"]}'
```
返回 JSON：`{"bizType":"...","bizRef":"...","digest":"...","timestamp":"...","createdAt":"..."}`

**③ 用 TxHash 查某笔交易**（系统链码 qscc）：

```bash
docker exec peer0.org1.example.com peer chaincode query \
  -C mychannel -n qscc \
  -c '{"Args":["GetTransactionByID","mychannel","<完整 64 位 TxHash>"]}'
```
返回 protobuf 序列化字节流（含完整背书签名）。

**④ 校验 digest 是否在链上**（VerifyDigest 自带零知识对比）：

```bash
docker exec peer0.org1.example.com peer chaincode query \
  -C mychannel -n anchorcc \
  -c '{"Args":["VerifyDigest","cross_auth","REQ-...","<digest hex>"]}'
```
返回 `true` 或 `false`。

**⑤ 拉取完整区块 protobuf**：

```bash
docker exec peer0.org1.example.com peer channel fetch 5 /tmp/block5.pb \
  -c mychannel -o orderer.example.com:7050 \
  --tls --cafile /etc/hyperledger/fabric/tls/ca.crt

# 拷出来用 configtxlator 解码
docker cp peer0.org1.example.com:/tmp/block5.pb ./block5.pb
```

### 10.4 通过数据库看链上数据副本（最快，但不是链原始数据）

`chain_anchors` 表保存了所有上链记录的本地副本（每笔上链时同步写入）：

```bash
docker exec bidm-mysql mysql -uroot -p6428734qwe iot_auth -e \
  "SELECT id, biz_type, biz_ref, tx_hash, block_height, created_at FROM chain_anchors ORDER BY id DESC LIMIT 20;"
```

或在 **MySQL Workbench** 里连接：

| 字段 | 值 |
|---|---|
| Hostname | `127.0.0.1` |
| **Port** | **`3307`** （宿主映射端口，不是 3306！） |
| Username | `root` |
| Password | `6428734qwe`（可用 `docker inspect bidm-mysql` 查） |
| Default Schema | `iot_auth` |

> ⚠ 这是**业务库副本**，不是链上原始数据。要证明数据真在链上，请走 10.2 的 `/api/chain/verify` 或 10.3 的 `peer chaincode query`。

### 10.5 推荐演示线（答辩 30 秒打通）

1. 打开 `/chain` 页面，让评委看到拓扑图 + 区块列表
2. 复制一笔 TxHash
3. 右下「链上读验证」粘贴 → 现场说：「这是后端去 Fabric qscc 系统链码实时查的，看到 Org1MSP+Org2MSP **双方签名背书**，验证码 VALID，链上 digest 与数据库 digest 一致」
4. 如果评委追问"你怎么确定不是后端伪造的" → 切到终端，跑一遍 10.3 ② `peer chaincode query ... anchorcc QueryAnchor ...`，直接拉同一条记录，digest 完全一致

## 11. 停机与清理

### 11.1 关闭 Fabric 网络

```bash
cd /Users/chenminggang/claude/trae_projects/iot-cross-domain/fabric
bash network_reset.sh
```

### 11.2 可选：删除 MySQL 容器

```bash
docker rm -f bidm-mysql
```

---

如需我再补一份“生产化部署文档”（Nginx 反代、HTTPS、systemd、数据库备份与日志轮转），我可以在本 README 继续追加生产章节。
