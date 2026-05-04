package oracle

import "math/rand"

type StatusReport struct {
	DeviceDID     string `json:"deviceDid"`
	Online        bool   `json:"online"`
	FirmwareValid bool   `json:"firmwareValid"`
	CertValid     bool   `json:"certValid"`
	Score         int    `json:"score"`
}

func Simulate(deviceDID string) StatusReport {
	online := rand.Intn(100) >= 5
	fw := rand.Intn(100) >= 10
	cert := rand.Intn(100) >= 8
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
