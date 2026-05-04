package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"iot-cross-domain/backend/internal/model"
	"iot-cross-domain/backend/internal/oracle"

	"gorm.io/gorm"
)

// AggregationResult collects everything we want to expose to UI / callers
// for a single multi-node oracle aggregation pass.
type AggregationResult struct {
	Update      *model.DeviceStateUpdate
	Submissions []model.OracleSubmission
	Threshold   int
	Aggregated  oracle.StatusReport
	State       string
	Severity    int
	Message     string
	Anchor      AnchorReceipt
	NoiseRate   int
	Reached     bool // whether participating count >= threshold
}

type AnchorReceipt struct {
	TxHash      string `json:"txHash"`
	BlockHeight uint64 `json:"blockHeight"`
}

// runOracleAggregation simulates one round of multi-node attestation, persists
// every per-node submission, computes majority vote, and (when threshold is met)
// anchors the aggregated result on Fabric.
func (h *Handler) runOracleAggregation(deviceDID string, noiseRate int) (*AggregationResult, error) {
	var device model.Device
	if err := h.DB.Where("device_d_id = ?", deviceDID).First(&device).Error; err != nil {
		return nil, fmt.Errorf("device not found")
	}

	// 1. fetch active oracle nodes
	var nodes []model.OracleNode
	_ = h.DB.Where("status = ?", "ACTIVE").Order("id asc").Find(&nodes).Error

	// 2. fetch threshold
	threshold := h.fetchOracleThreshold()

	// 3. ground truth + per-node noisy reports
	base := oracle.Simulate(deviceDID)
	subs := make([]model.OracleSubmission, 0, len(nodes))
	for _, n := range nodes {
		r := oracle.SimulateForNode(base, n.NodeName, noiseRate)
		subs = append(subs, model.OracleSubmission{
			DeviceDID:     deviceDID,
			NodeID:        n.ID,
			NodeName:      n.NodeName,
			Online:        r.Online,
			FirmwareValid: r.FirmwareValid,
			CertValid:     r.CertValid,
			Score:         r.Score,
			Signature:     oracle.FakeSign(n.NodeName, r),
		})
	}
	// If no active nodes registered, fall back to single-node simulation so the
	// system still functions on a fresh setup.
	if len(subs) == 0 {
		subs = append(subs, model.OracleSubmission{
			DeviceDID:     deviceDID,
			NodeID:        0,
			NodeName:      "fallback-node",
			Online:        base.Online,
			FirmwareValid: base.FirmwareValid,
			CertValid:     base.CertValid,
			Score:         base.Score,
			Signature:     oracle.FakeSign("fallback-node", base),
		})
	}

	// 4. majority vote per boolean field
	majOnline := majorityBool(subs, func(s model.OracleSubmission) bool { return s.Online })
	majFw := majorityBool(subs, func(s model.OracleSubmission) bool { return s.FirmwareValid })
	majCert := majorityBool(subs, func(s model.OracleSubmission) bool { return s.CertValid })
	avgScore := averageScore(subs)
	for i := range subs {
		// a submission is "in majority" if all 3 booleans match the majority
		if subs[i].Online == majOnline && subs[i].FirmwareValid == majFw && subs[i].CertValid == majCert {
			subs[i].InMajority = true
		}
	}

	aggregated := oracle.StatusReport{
		DeviceDID:     deviceDID,
		Online:        majOnline,
		FirmwareValid: majFw,
		CertValid:     majCert,
		Score:         avgScore,
	}

	stateLabel := "RISKY"
	if aggregated.Score >= 70 {
		stateLabel = "TRUSTED"
	}
	severity, msg := classifyReport(aggregated)

	reached := len(subs) >= threshold

	// 5. transactional persist: update device + insert update + insert submissions
	var update model.DeviceStateUpdate
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&device).Update("runtime_state", stateLabel).Error; err != nil {
			return err
		}
		update = model.DeviceStateUpdate{
			DeviceDID:          deviceDID,
			Online:             aggregated.Online,
			FirmwareValid:      aggregated.FirmwareValid,
			CertValid:          aggregated.CertValid,
			Score:              aggregated.Score,
			StateLabel:         stateLabel,
			Severity:           severity,
			Message:            msg,
			ParticipatingNodes: len(subs),
			ThresholdAtAgg:     threshold,
		}
		if err := tx.Create(&update).Error; err != nil {
			return err
		}
		for i := range subs {
			subs[i].AggregationID = update.ID
		}
		if err := tx.Create(&subs).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	res := &AggregationResult{
		Update:      &update,
		Submissions: subs,
		Threshold:   threshold,
		Aggregated:  aggregated,
		State:       stateLabel,
		Severity:    severity,
		Message:     msg,
		NoiseRate:   noiseRate,
		Reached:     reached,
	}

	// 6. anchor on Fabric only when threshold is met
	if reached {
		digestRaw := fmt.Sprintf("%s|%t|%t|%t|%d|n=%d|t=%d", deviceDID, aggregated.Online, aggregated.FirmwareValid, aggregated.CertValid, aggregated.Score, len(subs), threshold)
		sum := sha256.Sum256([]byte(digestRaw))
		digest := hex.EncodeToString(sum[:])
		anchored, anchorErr := h.Chain.Anchor("oracle_report", deviceDID, digest)
		if anchorErr == nil {
			_ = h.DB.Create(&model.ChainAnchor{
				BizType: "oracle_report", BizRef: deviceDID, Digest: digest,
				TxHash: anchored.TxHash, BlockHeight: anchored.BlockHeight,
			}).Error
			_ = h.DB.Model(&update).Updates(map[string]any{
				"tx_hash":      anchored.TxHash,
				"block_height": anchored.BlockHeight,
			}).Error
			update.TxHash = anchored.TxHash
			update.BlockHeight = anchored.BlockHeight
			res.Anchor = AnchorReceipt{TxHash: anchored.TxHash, BlockHeight: anchored.BlockHeight}
		}
	}

	return res, nil
}

func majorityBool(subs []model.OracleSubmission, getter func(model.OracleSubmission) bool) bool {
	t, f := 0, 0
	for _, s := range subs {
		if getter(s) {
			t++
		} else {
			f++
		}
	}
	return t >= f
}

func averageScore(subs []model.OracleSubmission) int {
	if len(subs) == 0 {
		return 0
	}
	sum := 0
	for _, s := range subs {
		sum += s.Score
	}
	return sum / len(subs)
}

func classifyReport(r oracle.StatusReport) (int, string) {
	if !r.Online {
		return 2, "设备离线（多数派投票）"
	}
	if !r.FirmwareValid || !r.CertValid {
		return 2, "固件或证书校验失败（多数派投票）"
	}
	if r.Score < 70 {
		return 1, "风险评分偏低"
	}
	return 0, "OK"
}

func (h *Handler) fetchOracleThreshold() int {
	var ss model.SystemSetting
	if err := h.DB.Where("conf_key = ?", "oracle_threshold").First(&ss).Error; err != nil {
		return 1
	}
	v, err := strconv.Atoi(ss.ConfValue)
	if err != nil || v < 1 {
		return 1
	}
	return v
}
