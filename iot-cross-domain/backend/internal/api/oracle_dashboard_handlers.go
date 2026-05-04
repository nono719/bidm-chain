package api

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"
	"time"

	"iot-cross-domain/backend/internal/model"
	"iot-cross-domain/backend/pkg/response"

	"github.com/gin-gonic/gin"
)

// =============================================================
// GET /api/oracle/dashboard
// Returns: nodes (with last-seen + algorithm), threshold,
// recent aggregation success rate, algorithm distribution,
// per-node participation count for the last 7 days.
// =============================================================

func (h *Handler) OracleDashboard(c *gin.Context) {
	var nodes []model.OracleNode
	_ = h.DB.Order("id asc").Find(&nodes).Error

	threshold := h.fetchOracleThreshold()

	// Last seen per node = latest submission timestamp
	lastSeenByNode := map[uint]time.Time{}
	type lastSeenRow struct {
		NodeID  uint
		LastSeen time.Time
	}
	rows := []lastSeenRow{}
	_ = h.DB.Raw(`SELECT node_id AS node_id, MAX(created_at) AS last_seen FROM oracle_submissions GROUP BY node_id`).Scan(&rows).Error
	for _, r := range rows {
		lastSeenByNode[r.NodeID] = r.LastSeen
	}

	// 7-day participation count + majority count per node
	type partRow struct {
		NodeID         uint
		Total          int64
		MajorityCount  int64
	}
	prows := []partRow{}
	_ = h.DB.Raw(`SELECT node_id AS node_id, COUNT(*) AS total, SUM(CASE WHEN in_majority THEN 1 ELSE 0 END) AS majority_count FROM oracle_submissions WHERE created_at > ? GROUP BY node_id`, time.Now().Add(-7*24*time.Hour)).Scan(&prows).Error
	partByNode := map[uint]partRow{}
	for _, r := range prows {
		partByNode[r.NodeID] = r
	}

	// Algorithm distribution
	algoCount := map[string]int{}
	nodesOut := []gin.H{}
	for _, n := range nodes {
		algo := detectKeyAlgorithm(n.PublicKey)
		algoCount[algo]++

		ls := lastSeenByNode[n.ID]
		health := nodeHealth(n.Status, ls)
		participation := partByNode[n.ID]

		nodesOut = append(nodesOut, gin.H{
			"id":            n.ID,
			"nodeName":      n.NodeName,
			"status":        n.Status,
			"algorithm":     algo,
			"health":        health,
			"lastSeen":      nilIfZeroTime(ls),
			"createdAt":     n.CreatedAt,
			"updatedAt":     n.UpdatedAt,
			"participation": participation.Total,
			"majorityCount": participation.MajorityCount,
		})
	}

	// Aggregation success rate over last 24h: rounds with reachedThreshold / total rounds
	since := time.Now().Add(-24 * time.Hour)
	var totalRounds int64
	var trustedRounds int64
	_ = h.DB.Model(&model.DeviceStateUpdate{}).Where("created_at > ?", since).Count(&totalRounds).Error
	_ = h.DB.Model(&model.DeviceStateUpdate{}).Where("created_at > ? AND state_label = ?", since, "TRUSTED").Count(&trustedRounds).Error
	successRate := 0.0
	if totalRounds > 0 {
		successRate = float64(trustedRounds) / float64(totalRounds)
	}

	// Latest aggregation timestamp + tx hash
	var lastUpdate model.DeviceStateUpdate
	_ = h.DB.Order("id DESC").Limit(1).First(&lastUpdate).Error

	response.OK(c, gin.H{
		"nodes":           nodesOut,
		"threshold":       threshold,
		"nodesTotal":      len(nodes),
		"nodesActive":     countActive(nodes),
		"algorithmDist":   algoCount,
		"successRate":     successRate,
		"roundsTotal24h":  totalRounds,
		"roundsTrusted24h": trustedRounds,
		"lastAggregation": gin.H{
			"id":          lastUpdate.ID,
			"deviceDid":   lastUpdate.DeviceDID,
			"stateLabel":  lastUpdate.StateLabel,
			"score":       lastUpdate.Score,
			"createdAt":   nilIfZeroTime(lastUpdate.CreatedAt),
			"txHash":      lastUpdate.TxHash,
			"blockHeight": lastUpdate.BlockHeight,
		},
	})
}

// =============================================================
// GET /api/oracle/aggregations?limit=N
// Returns recent multi-node aggregation rounds with submission counts
// (without full submission detail to keep payload small).
// =============================================================

func (h *Handler) OracleAggregations(c *gin.Context) {
	limit := 30
	if v, _ := strconv.Atoi(c.Query("limit")); v > 0 && v <= 200 {
		limit = v
	}
	deviceDID := strings.TrimSpace(c.Query("deviceDid"))

	q := h.DB.Model(&model.DeviceStateUpdate{}).Order("id DESC").Limit(limit)
	if deviceDID != "" {
		q = q.Where("device_d_id = ?", deviceDID)
	}
	var ups []model.DeviceStateUpdate
	if err := q.Find(&ups).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// Batch-fetch submission counts
	type cnt struct {
		AggregationID uint
		Total         int64
		Majority      int64
	}
	ids := make([]uint, len(ups))
	for i, u := range ups {
		ids[i] = u.ID
	}
	cmap := map[uint]cnt{}
	if len(ids) > 0 {
		var rows []cnt
		_ = h.DB.Raw(`SELECT aggregation_id AS aggregation_id, COUNT(*) AS total, SUM(CASE WHEN in_majority THEN 1 ELSE 0 END) AS majority FROM oracle_submissions WHERE aggregation_id IN ? GROUP BY aggregation_id`, ids).Scan(&rows).Error
		for _, r := range rows {
			cmap[r.AggregationID] = r
		}
	}

	out := make([]gin.H, 0, len(ups))
	for _, u := range ups {
		c := cmap[u.ID]
		out = append(out, gin.H{
			"id":                 u.ID,
			"deviceDid":          u.DeviceDID,
			"online":             u.Online,
			"firmwareValid":      u.FirmwareValid,
			"certValid":          u.CertValid,
			"score":              u.Score,
			"stateLabel":         u.StateLabel,
			"severity":           u.Severity,
			"message":            u.Message,
			"txHash":             u.TxHash,
			"blockHeight":        u.BlockHeight,
			"participatingNodes": u.ParticipatingNodes,
			"thresholdAtAgg":     u.ThresholdAtAgg,
			"submissionsTotal":   c.Total,
			"submissionsInMaj":   c.Majority,
			"createdAt":          u.CreatedAt,
		})
	}
	response.OK(c, out)
}

// =============================================================
// GET /api/oracle/aggregations/:id  - full detail with all submissions
// =============================================================

func (h *Handler) OracleAggregationDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var u model.DeviceStateUpdate
	if err := h.DB.First(&u, uint(id)).Error; err != nil {
		response.BadRequest(c, "aggregation not found")
		return
	}
	var subs []model.OracleSubmission
	_ = h.DB.Where("aggregation_id = ?", u.ID).Order("id ASC").Find(&subs).Error

	// Vote breakdown for the 3 boolean fields + score distribution
	totalSubs := len(subs)
	online := 0
	fw := 0
	cert := 0
	scores := []int{}
	for _, s := range subs {
		if s.Online {
			online++
		}
		if s.FirmwareValid {
			fw++
		}
		if s.CertValid {
			cert++
		}
		scores = append(scores, s.Score)
	}
	resp := gin.H{
		"aggregation": gin.H{
			"id":                 u.ID,
			"deviceDid":          u.DeviceDID,
			"online":             u.Online,
			"firmwareValid":      u.FirmwareValid,
			"certValid":          u.CertValid,
			"score":              u.Score,
			"stateLabel":         u.StateLabel,
			"severity":           u.Severity,
			"message":            u.Message,
			"txHash":             u.TxHash,
			"blockHeight":        u.BlockHeight,
			"participatingNodes": u.ParticipatingNodes,
			"thresholdAtAgg":     u.ThresholdAtAgg,
			"createdAt":          u.CreatedAt,
		},
		"submissions": subs,
		"votes": gin.H{
			"total":    totalSubs,
			"online":   gin.H{"yes": online, "no": totalSubs - online},
			"firmware": gin.H{"yes": fw, "no": totalSubs - fw},
			"cert":     gin.H{"yes": cert, "no": totalSubs - cert},
			"scores":   scores,
		},
	}
	response.OK(c, resp)
}

// =============================================================
// POST /api/oracle/demo/simulate  (ADMIN only) - run one aggregation
// for a device to demo the full multi-node + threshold + chain flow.
// Body: { deviceDid: string, noiseRate?: int }
// =============================================================

func (h *Handler) OracleDemoSimulate(c *gin.Context) {
	if c.GetString("role") != "ADMIN" {
		response.Forbidden(c, "ADMIN role required")
		return
	}
	var req struct {
		DeviceDID string `json:"deviceDid"`
		NoiseRate int    `json:"noiseRate"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.DeviceDID == "" {
		response.BadRequest(c, "deviceDid required")
		return
	}
	if req.NoiseRate <= 0 || req.NoiseRate > 80 {
		req.NoiseRate = 15
	}
	res, err := h.runOracleAggregation(req.DeviceDID, req.NoiseRate)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	chainReceipt := gin.H{}
	if res.Anchor.TxHash != "" {
		chainReceipt = h.enrichChainReceipt(res.Anchor.TxHash, res.Anchor.BlockHeight)
	}
	response.OK(c, gin.H{
		"aggregationId":    res.Update.ID,
		"deviceDid":        req.DeviceDID,
		"noiseRate":        req.NoiseRate,
		"submissions":      res.Submissions,
		"threshold":        res.Threshold,
		"participating":    len(res.Submissions),
		"reachedThreshold": res.Reached,
		"aggregated":       res.Aggregated,
		"state":            res.State,
		"severity":         res.Severity,
		"message":          res.Message,
		"chain":            chainReceipt,
	})
}

// =============================================================
// helpers
// =============================================================

func detectKeyAlgorithm(pemPubKey string) string {
	block, _ := pem.Decode([]byte(pemPubKey))
	if block == nil {
		return "UNKNOWN"
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "UNKNOWN"
	}
	switch pub.(type) {
	case *rsaPubKeyMarker:
		return "RSA"
	}
	// avoid heavy imports — string-match the type name
	t := fmt.Sprintf("%T", pub)
	if strings.Contains(t, "rsa.PublicKey") {
		return "RSA"
	}
	if strings.Contains(t, "ecdsa.PublicKey") {
		return "ECDSA"
	}
	if strings.Contains(t, "ed25519.PublicKey") {
		return "ED25519"
	}
	return "UNKNOWN"
}

type rsaPubKeyMarker struct{} // unused; kept to avoid breaking x509 type-switch fallthrough above

func nodeHealth(status string, lastSeen time.Time) string {
	if status != "ACTIVE" {
		return strings.ToUpper(status)
	}
	if lastSeen.IsZero() {
		return "IDLE"
	}
	d := time.Since(lastSeen)
	if d < 5*time.Minute {
		return "HEALTHY"
	}
	if d < 30*time.Minute {
		return "LAGGING"
	}
	return "STALE"
}

func countActive(ns []model.OracleNode) int {
	c := 0
	for _, n := range ns {
		if n.Status == "ACTIVE" {
			c++
		}
	}
	return c
}

func nilIfZeroTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
