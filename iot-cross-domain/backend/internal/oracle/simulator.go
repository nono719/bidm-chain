package oracle

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
)

type StatusReport struct {
	DeviceDID     string `json:"deviceDid"`
	Online        bool   `json:"online"`
	FirmwareValid bool   `json:"firmwareValid"`
	CertValid     bool   `json:"certValid"`
	Score         int    `json:"score"`
}

// Simulate generates a baseline status report for a device.
// Used as the "ground truth" before per-node noise is applied.
func Simulate(deviceDID string) StatusReport {
	online := mrand.Intn(100) >= 5
	fw := mrand.Intn(100) >= 10
	cert := mrand.Intn(100) >= 8
	score := 0
	if online {
		score += 40
	}
	if fw {
		score += 30
	}
	if cert {
		score += 30
	}
	return StatusReport{
		DeviceDID:     deviceDID,
		Online:        online,
		FirmwareValid: fw,
		CertValid:     cert,
		Score:         score,
	}
}

// SimulateForNode applies tiny per-node noise so different oracle nodes
// produce slightly different observations of the same device, exercising
// the threshold-aggregation/majority-vote path.
//
// noiseRate: percentage chance any single boolean flips (0-100).
func SimulateForNode(base StatusReport, nodeName string, noiseRate int) StatusReport {
	out := base
	flip := func() bool { return mrand.Intn(100) < noiseRate }
	if flip() {
		out.Online = !out.Online
	}
	if flip() {
		out.FirmwareValid = !out.FirmwareValid
	}
	if flip() {
		out.CertValid = !out.CertValid
	}
	out.Score = 0
	if out.Online {
		out.Score += 40
	}
	if out.FirmwareValid {
		out.Score += 30
	}
	if out.CertValid {
		out.Score += 30
	}
	// jitter score by ±3
	out.Score += mrand.Intn(7) - 3
	if out.Score < 0 {
		out.Score = 0
	}
	if out.Score > 100 {
		out.Score = 100
	}
	return out
}

// FakeSign generates a deterministic-looking signature hex string for demo
// purposes (real Fabric endorsements are still produced separately at chain
// anchor time; this represents the per-node attestation over the report).
func FakeSign(nodeName string, report StatusReport) string {
	salt := make([]byte, 8)
	_, _ = rand.Read(salt)
	payload := fmt.Sprintf("%s|%s|%t|%t|%t|%d|%x", nodeName, report.DeviceDID, report.Online, report.FirmwareValid, report.CertValid, report.Score, salt)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
