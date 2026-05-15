package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"iot-cross-domain/backend/internal/model"
	"iot-cross-domain/backend/pkg/response"

	"github.com/gin-gonic/gin"
)

// ============================================================
// Fault-tolerance demo for the Fabric peer/orderer containers.
//
// Lets an ADMIN flip the running state of a specific Fabric container
// (whitelisted to the three components we actually use) and then trigger
// a test anchor to *observe* the consequence on the chain. This is what
// makes the consortium chain story tangible during a defence:
//
//   1. Stop orderer  → submit fails (no consensus possible)
//   2. Stop peer0.org1.example.com → gateway connect refused
//   3. Stop peer0.org2.example.com → may succeed or fail depending on
//      endorsement policy (lets you discuss redundancy)
// ============================================================

type fabricContainer struct {
	Role        string
	Endpoint    string
	Description string
}

var allowedFabricContainers = map[string]fabricContainer{
	"peer0.org1.example.com": {Role: "peer", Endpoint: "localhost:7051", Description: "Org1 背书节点（Gateway 入口）"},
	"peer0.org2.example.com": {Role: "peer", Endpoint: "localhost:9051", Description: "Org2 背书节点"},
	"orderer.example.com":    {Role: "orderer", Endpoint: "localhost:7050", Description: "Raft 排序节点"},
}

type peerStatus struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Endpoint    string `json:"endpoint"`
	Description string `json:"description"`
	Running     bool   `json:"running"`
	State       string `json:"state"`
}

func (h *Handler) ChainDemoPeerStatus(c *gin.Context) {
	out := []peerStatus{}
	// preserve order: peer0.org1, peer0.org2, orderer (deterministic for UI)
	for _, name := range []string{"peer0.org1.example.com", "peer0.org2.example.com", "orderer.example.com"} {
		info := allowedFabricContainers[name]
		state := dockerInspectState(name)
		out = append(out, peerStatus{
			Name: name, Role: info.Role, Endpoint: info.Endpoint, Description: info.Description,
			Running: state == "running", State: state,
		})
	}
	response.OK(c, out)
}

func dockerInspectState(name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Status}}", name)
	b, err := cmd.Output()
	if err != nil {
		return "missing"
	}
	return strings.TrimSpace(string(b))
}

type peerOpReq struct {
	Name string `json:"name" binding:"required"`
}

func (h *Handler) ChainDemoPeerStop(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "admin only")
		return
	}
	var req peerOpReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "name required")
		return
	}
	info, ok := allowedFabricContainers[req.Name]
	if !ok {
		response.BadRequest(c, "container not in whitelist")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "stop", req.Name)
	out, err := cmd.CombinedOutput()
	stateAfter := dockerInspectState(req.Name)
	if err != nil {
		_ = h.auditEx(model.AuditLog{
			Module: "chain", Action: "demo_peer_stop", Operator: c.GetString("username"),
			Result: "FAIL", Message: req.Name + ": " + strings.TrimSpace(string(out)),
		})
		response.InternalError(c, "docker stop failed: "+strings.TrimSpace(string(out)))
		return
	}
	_ = h.auditEx(model.AuditLog{
		Module: "chain", Action: "demo_peer_stop", Operator: c.GetString("username"),
		Result: "OK", Message: req.Name,
	})
	response.OK(c, gin.H{
		"name": req.Name, "role": info.Role, "action": "stopped",
		"state": stateAfter, "running": stateAfter == "running",
	})
}

func (h *Handler) ChainDemoPeerStart(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "admin only")
		return
	}
	var req peerOpReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "name required")
		return
	}
	info, ok := allowedFabricContainers[req.Name]
	if !ok {
		response.BadRequest(c, "container not in whitelist")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "start", req.Name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = h.auditEx(model.AuditLog{
			Module: "chain", Action: "demo_peer_start", Operator: c.GetString("username"),
			Result: "FAIL", Message: req.Name + ": " + strings.TrimSpace(string(out)),
		})
		response.InternalError(c, "docker start failed: "+strings.TrimSpace(string(out)))
		return
	}
	// Wait for the service inside the container to be reachable, then for
	// the gateway peer's service-discovery cache to refresh. Orderer in
	// particular needs ~10s: container reaches "running" instantly, gRPC
	// becomes available a few seconds later, then peer0.org1's discovery
	// (which is what fabric-gateway uses to find an orderer) re-learns the
	// orderer is back. If we return too early the next anchor will see
	// "no orderers could successfully process transaction".
	time.Sleep(12 * time.Second)
	stateAfter := dockerInspectState(req.Name)
	_ = h.auditEx(model.AuditLog{
		Module: "chain", Action: "demo_peer_start", Operator: c.GetString("username"),
		Result: "OK", Message: req.Name,
	})
	response.OK(c, gin.H{
		"name": req.Name, "role": info.Role, "action": "started",
		"state": stateAfter, "running": stateAfter == "running",
	})
}

// ChainDemoTestAnchor issues a throw-away anchor request so the UI can
// observe whether the chain currently accepts writes. Uses biz_type
// "demo_fault_test" so production queries don't get polluted.
func (h *Handler) ChainDemoTestAnchor(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "admin only")
		return
	}
	ts := time.Now()
	digestRaw := fmt.Sprintf("fault-demo-%d", ts.UnixNano())
	sum := sha256.Sum256([]byte(digestRaw))
	digest := hex.EncodeToString(sum[:])
	bizRef := "demo-" + strconv.FormatInt(ts.Unix(), 10)

	start := time.Now()
	// Use AnchorOpOnChain so the resulting record is also written to
	// chain_anchors — keeps the "累计上链记录" counter in sync with the
	// on-chain block height for demo purposes.
	anchor, err := h.AnchorOpOnChain("demo_fault_test", bizRef, digest)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		_ = h.auditEx(model.AuditLog{
			Module: "chain", Action: "demo_test_anchor", Operator: c.GetString("username"),
			Result: "FAIL", Message: err.Error(),
		})
		response.OK(c, gin.H{
			"ok":         false,
			"durationMs": duration,
			"error":      err.Error(),
			"hint":       diagnoseAnchorError(err.Error()),
		})
		return
	}
	receipt := h.enrichChainReceipt(anchor.TxHash, anchor.BlockHeight)
	_ = h.auditEx(model.AuditLog{
		Module: "chain", Action: "demo_test_anchor", Operator: c.GetString("username"),
		Result: "OK", Contract: "anchorcc", Method: "AnchorRecord",
		TxHash: anchor.TxHash, BlockHeight: anchor.BlockHeight,
		Message: fmt.Sprintf("test anchor succeeded in %dms", duration),
	})
	response.OK(c, gin.H{
		"ok":          true,
		"durationMs":  duration,
		"txHash":      anchor.TxHash,
		"blockHeight": anchor.BlockHeight,
		"chain":       receipt,
	})
}

// diagnoseAnchorError gives the UI a friendly Chinese hint based on
// typical Fabric error fingerprints.
func diagnoseAnchorError(msg string) string {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "connection refused") || strings.Contains(low, "unavailable"):
		return "无法连接背书节点 / 排序节点（容器已停？）"
	case strings.Contains(low, "deadline exceeded") || strings.Contains(low, "timeout"):
		return "等待背书 / 排序超时（节点失联或网络异常）"
	case strings.Contains(low, "endorsement") || strings.Contains(low, "endorse"):
		return "背书阶段失败 — 可能的背书节点已停"
	case strings.Contains(low, "orderer"):
		return "排序节点不可用（停 orderer 后无法生成新区块）"
	case strings.Contains(low, "no peers") || strings.Contains(low, "no endorsement"):
		return "找不到可用的背书节点"
	}
	return "上链失败 — 检查 docker ps 与节点状态"
}
