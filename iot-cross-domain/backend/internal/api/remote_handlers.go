package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"iot-cross-domain/backend/internal/chain"
	"iot-cross-domain/backend/internal/middleware"
	"iot-cross-domain/backend/internal/model"
	"iot-cross-domain/backend/pkg/response"

	"github.com/gin-gonic/gin"
)

// ============================================================
// Remote read endpoints — query chain-recorded data via cross-domain token
// ============================================================

func (h *Handler) RemoteProfile(c *gin.Context) {
	// Prefer the resolved operation-target DID set by the cross-domain
	// middleware (honours X-Target-Device-DID); fall back to the raw
	// X-Device-DID header if the middleware didn't run (defensive).
	deviceDID := c.GetString("crossOpDeviceDid")
	if deviceDID == "" {
		deviceDID = middleware.HeaderDecoded(c, "X-Device-DID")
	}
	if deviceDID == "" {
		response.BadRequest(c, "X-Device-DID required")
		return
	}
	var device model.Device
	if err := h.DB.Where("device_d_id = ?", deviceDID).First(&device).Error; err != nil {
		response.BadRequest(c, "device not found")
		return
	}
	var lastState model.DeviceStateUpdate
	_ = h.DB.Where("device_d_id = ?", deviceDID).Order("id DESC").First(&lastState).Error

	var anchorCount int64
	_ = h.DB.Model(&model.ChainAnchor{}).Where("biz_ref = ?", deviceDID).Count(&anchorCount).Error

	_ = h.auditEx(model.AuditLog{
		Module:     "operation",
		Action:     "remote_read_profile",
		Operator:   c.GetString("username"),
		Result:     "OK",
		SubjectDID: deviceDID,
		Message:    "remote profile read via cross-domain token",
	})

	response.OK(c, gin.H{
		"device": gin.H{
			"deviceDid":    device.DeviceDID,
			"displayName":  device.DisplayName,
			"deviceType":   device.DeviceType,
			"domainCode":   device.DomainCode,
			"lifecycle":    device.Lifecycle,
			"runtimeState": device.RuntimeState,
			"metadataJson": device.MetadataJSON,
			"createdAt":    device.CreatedAt,
			"updatedAt":    device.UpdatedAt,
		},
		"lastState": gin.H{
			"online":        lastState.Online,
			"firmwareValid": lastState.FirmwareValid,
			"certValid":     lastState.CertValid,
			"score":         lastState.Score,
			"stateLabel":    lastState.StateLabel,
			"severity":      lastState.Severity,
			"message":       lastState.Message,
			"txHash":        lastState.TxHash,
			"blockHeight":   lastState.BlockHeight,
			"reportedAt":    lastState.CreatedAt,
		},
		"anchorCount": anchorCount,
	})
}

func (h *Handler) RemoteTelemetry(c *gin.Context) {
	// Prefer the resolved operation-target DID set by the cross-domain
	// middleware (honours X-Target-Device-DID); fall back to the raw
	// X-Device-DID header if the middleware didn't run (defensive).
	deviceDID := c.GetString("crossOpDeviceDid")
	if deviceDID == "" {
		deviceDID = middleware.HeaderDecoded(c, "X-Device-DID")
	}
	if deviceDID == "" {
		response.BadRequest(c, "X-Device-DID required")
		return
	}
	limit := 30
	if v, _ := strconv.Atoi(c.Query("limit")); v > 0 && v <= 200 {
		limit = v
	}
	var rows []model.DeviceStateUpdate
	if err := h.DB.Where("device_d_id = ?", deviceDID).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	_ = h.auditEx(model.AuditLog{
		Module:     "operation",
		Action:     "remote_read_telemetry",
		Operator:   c.GetString("username"),
		Result:     "OK",
		SubjectDID: deviceDID,
		Message:    fmt.Sprintf("read %d telemetry rows", len(rows)),
	})

	response.OK(c, gin.H{"items": rows, "count": len(rows)})
}

func (h *Handler) RemoteAuditTrail(c *gin.Context) {
	// Prefer the resolved operation-target DID set by the cross-domain
	// middleware (honours X-Target-Device-DID); fall back to the raw
	// X-Device-DID header if the middleware didn't run (defensive).
	deviceDID := c.GetString("crossOpDeviceDid")
	if deviceDID == "" {
		deviceDID = middleware.HeaderDecoded(c, "X-Device-DID")
	}
	if deviceDID == "" {
		response.BadRequest(c, "X-Device-DID required")
		return
	}
	limit := 50
	if v, _ := strconv.Atoi(c.Query("limit")); v > 0 && v <= 200 {
		limit = v
	}
	var anchors []model.ChainAnchor
	if err := h.DB.Where("biz_ref = ?", deviceDID).Order("block_height DESC, id DESC").Limit(limit).Find(&anchors).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	_ = h.auditEx(model.AuditLog{
		Module:     "operation",
		Action:     "remote_read_audit",
		Operator:   c.GetString("username"),
		Result:     "OK",
		SubjectDID: deviceDID,
		Message:    fmt.Sprintf("read %d chain anchors", len(anchors)),
	})

	response.OK(c, gin.H{"items": anchors, "count": len(anchors)})
}

// ============================================================
// Operation effect — real state mutation for write operations
// ============================================================

type opEffectResult struct {
	StateBefore   string         `json:"stateBefore"`
	StateAfter    string         `json:"stateAfter"`
	StateRevertAt *time.Time     `json:"stateRevertAt,omitempty"`
	MetadataPatch map[string]any `json:"metadataPatch,omitempty"`
	Description   string         `json:"description"`
}

// applyOperationEffect mutates device state for write operations.
// Read-only ops (READ_*) return nil with no error.
func (h *Handler) applyOperationEffect(op string, device *model.Device, payload string) (*opEffectResult, error) {
	switch op {
	case "RESTART_DEVICE":
		before := device.RuntimeState
		now := time.Now()
		revert := now.Add(5 * time.Second)
		if err := h.DB.Model(device).Update("runtime_state", "RESTARTING").Error; err != nil {
			return nil, err
		}
		go h.scheduleStateRevert(device.DeviceDID, "TRUSTED", revert)
		return &opEffectResult{
			StateBefore:   before,
			StateAfter:    "RESTARTING",
			StateRevertAt: &revert,
			Description:   "device entering RESTARTING for 5s, will return to TRUSTED",
		}, nil

	case "WRITE_DEVICE_CONFIG":
		patch, err := mergeMetadata(device.MetadataJSON, payload)
		if err != nil {
			return nil, err
		}
		newMeta, _ := json.Marshal(patch)
		if err := h.DB.Model(device).Update("metadata_json", string(newMeta)).Error; err != nil {
			return nil, err
		}
		return &opEffectResult{
			StateBefore:   device.RuntimeState,
			StateAfter:    device.RuntimeState,
			MetadataPatch: patch,
			Description:   fmt.Sprintf("metadata merged with %d field(s) of patch", len(patch)),
		}, nil

	case "ADMIN_FIRMWARE_UPGRADE":
		before := device.RuntimeState
		patch, _ := mergeMetadata(device.MetadataJSON, payload)
		// auto-bump firmware version if not provided
		if _, ok := patch["firmware"]; !ok {
			cur, _ := patch["firmwareVersion"].(string)
			patch["firmwareVersion"] = bumpVersion(cur)
		}
		if v, ok := patch["firmware"].(string); ok {
			patch["firmwareVersion"] = v
		}
		patch["lastUpgradeAt"] = time.Now().Format(time.RFC3339)
		newMeta, _ := json.Marshal(patch)
		now := time.Now()
		revert := now.Add(10 * time.Second)
		if err := h.DB.Model(device).Updates(map[string]any{
			"runtime_state": "UPGRADING",
			"metadata_json": string(newMeta),
		}).Error; err != nil {
			return nil, err
		}
		go h.scheduleStateRevert(device.DeviceDID, "TRUSTED", revert)
		return &opEffectResult{
			StateBefore:   before,
			StateAfter:    "UPGRADING",
			StateRevertAt: &revert,
			MetadataPatch: patch,
			Description:   "device entering UPGRADING for 10s, firmware bumped",
		}, nil

	case "READ_REMOTE_PROFILE", "READ_TELEMETRY":
		return &opEffectResult{
			StateBefore: device.RuntimeState,
			StateAfter:  device.RuntimeState,
			Description: "read-only op, no mutation",
		}, nil

	default:
		return &opEffectResult{
			StateBefore: device.RuntimeState,
			StateAfter:  device.RuntimeState,
			Description: "unmapped op, recorded only",
		}, nil
	}
}

func (h *Handler) scheduleStateRevert(deviceDid string, target string, _ time.Time) {
	wait := 5 * time.Second
	if target == "TRUSTED" {
		// Use longer wait if comes from UPGRADING; let's check current state
	}
	time.Sleep(wait)
	var dev model.Device
	if err := h.DB.Where("device_d_id = ?", deviceDid).First(&dev).Error; err != nil {
		return
	}
	if dev.RuntimeState == "UPGRADING" {
		time.Sleep(5 * time.Second)
	}
	_ = h.DB.Model(&model.Device{}).Where("device_d_id = ?", deviceDid).Update("runtime_state", target).Error
}

func mergeMetadata(existingJSON string, patchJSON string) (map[string]any, error) {
	out := map[string]any{}
	if strings.TrimSpace(existingJSON) != "" {
		_ = json.Unmarshal([]byte(existingJSON), &out)
	}
	if strings.TrimSpace(patchJSON) == "" {
		return out, nil
	}
	patch := map[string]any{}
	if err := json.Unmarshal([]byte(patchJSON), &patch); err != nil {
		return out, fmt.Errorf("invalid payload JSON: %w", err)
	}
	for k, v := range patch {
		out[k] = v
	}
	return out, nil
}

func bumpVersion(cur string) string {
	if cur == "" {
		return "v1.0.1"
	}
	// naive bump: strip leading 'v', split by '.', bump last segment
	s := strings.TrimPrefix(cur, "v")
	parts := strings.Split(s, ".")
	if len(parts) == 0 {
		return "v1.0.1"
	}
	last := parts[len(parts)-1]
	n, _ := strconv.Atoi(last)
	parts[len(parts)-1] = strconv.Itoa(n + 1)
	return "v" + strings.Join(parts, ".")
}

// ============================================================
// Chain receipt enrichment — return endorsers in op response
// ============================================================

func (h *Handler) enrichChainReceipt(txHash string, blockHeight uint64) gin.H {
	out := gin.H{"txHash": txHash, "blockHeight": blockHeight}
	tx, err := h.Chain.TransactionByID(txHash)
	if err != nil || tx == nil {
		return out
	}
	endorsers := []gin.H{}
	for _, e := range tx.Endorsers {
		endorsers = append(endorsers, gin.H{
			"mspId":      e.MSPID,
			"commonName": e.CommonName,
		})
	}
	out["endorsers"] = endorsers
	out["validationCode"] = tx.ValidationCode
	out["validationMessage"] = tx.ValidationMessage
	if tx.Creator != nil {
		out["creator"] = gin.H{"mspId": tx.Creator.MSPID, "commonName": tx.Creator.CommonName}
	}
	return out
}

func opDigest(deviceDid, op, payload string, ts int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d", deviceDid, op, payload, ts)))
	return hex.EncodeToString(sum[:])
}

// AnchorOpOnChain anchors a write operation on Fabric and returns the receipt.
func (h *Handler) AnchorOpOnChain(bizType, bizRef, payload string) (*chain.AnchorResult, error) {
	digest := opDigest(bizRef, bizType, payload, time.Now().Unix())
	res, err := h.Chain.Anchor(bizType, bizRef, digest)
	if err != nil {
		return nil, err
	}
	_ = h.DB.Create(&model.ChainAnchor{
		BizType:     bizType,
		BizRef:      bizRef,
		Digest:      digest,
		TxHash:      res.TxHash,
		BlockHeight: res.BlockHeight,
	}).Error
	return &res, nil
}

// ============================================================
// CrossTargetDevices — list devices in the target domain that this
// operator can address with the current VERIFIED session.
// Used by the RemoteConsole's "target device" picker so a domain-a
// operator can pick which domain-b device to operate against.
// ============================================================

func (h *Handler) CrossTargetDevices(c *gin.Context) {
	deviceDID := strings.TrimSpace(c.Query("deviceDid"))
	targetDomain := strings.TrimSpace(c.Query("targetDomain"))
	if deviceDID == "" || targetDomain == "" {
		response.BadRequest(c, "deviceDid and targetDomain required")
		return
	}
	session, reason := h.latestUsableVerifiedSession(deviceDID, targetDomain)
	if session == nil {
		response.Forbidden(c, "no usable session: "+reason)
		return
	}
	var devs []model.Device
	if err := h.DB.Where("domain_code = ? AND lifecycle = ?", targetDomain, "ACTIVE").
		Order("id desc").Find(&devs).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	out := make([]gin.H, 0, len(devs))
	for _, d := range devs {
		out = append(out, gin.H{
			"deviceDid":    d.DeviceDID,
			"displayName":  d.DisplayName,
			"deviceType":   d.DeviceType,
			"runtimeState": d.RuntimeState,
			"lifecycle":    d.Lifecycle,
			"domainCode":   d.DomainCode,
		})
	}
	response.OK(c, gin.H{
		"sessionRequestId": session.RequestID,
		"toDomain":         session.ToDomainCode,
		"items":            out,
		"count":            len(out),
	})
}

// ============================================================
// CrossSessionPing — quick endpoint to fetch the current session info for the operator
// (used by RemoteConsole UI to populate the AuthToken card)
// ============================================================

func (h *Handler) CrossActiveSession(c *gin.Context) {
	deviceDID := strings.TrimSpace(c.Query("deviceDid"))
	targetDomain := strings.TrimSpace(c.Query("targetDomain"))
	if deviceDID == "" || targetDomain == "" {
		response.BadRequest(c, "deviceDid and targetDomain required")
		return
	}
	session, reason := h.latestUsableVerifiedSession(deviceDID, targetDomain)
	if session == nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"active": false, "reason": reason}})
		return
	}
	now := time.Now()
	var ttlSec int64 = 0
	if session.ExpiresAt != nil {
		ttlSec = int64(session.ExpiresAt.Sub(now).Seconds())
		if ttlSec < 0 {
			ttlSec = 0
		}
	}
	response.OK(c, gin.H{
		"active":      true,
		"requestId":   session.RequestID,
		"deviceDid":   session.DeviceDID,
		"fromDomain":  session.FromDomainCode,
		"toDomain":    session.ToDomainCode,
		"resource":    session.Resource,
		"permission":  session.Permission,
		"verifiedAt":  session.VerifiedAt,
		"expiresAt":   session.ExpiresAt,
		"ttlSeconds":  ttlSec,
		"approvedBy":  session.ApprovedBy,
		"requestedBy": session.RequestedBy,
	})
}
