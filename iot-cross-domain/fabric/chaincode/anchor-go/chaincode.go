package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type SmartContract struct {
	contractapi.Contract
}

type AnchorRecord struct {
	BizType   string `json:"bizType"`
	BizRef    string `json:"bizRef"`
	Digest    string `json:"digest"`
	Timestamp string `json:"timestamp"`
	CreatedAt string `json:"createdAt"`
}

func (s *SmartContract) AnchorRecord(ctx contractapi.TransactionContextInterface, bizType, bizRef, digest, timestamp string) error {
	if bizType == "" || bizRef == "" || digest == "" {
		return fmt.Errorf("bizType, bizRef, digest are required")
	}
	key := anchorKey(bizType, bizRef)
	record := AnchorRecord{
		BizType:   bizType,
		BizRef:    bizRef,
		Digest:    digest,
		Timestamp: timestamp,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return ctx.GetStub().PutState(key, raw)
}

func (s *SmartContract) QueryAnchor(ctx contractapi.TransactionContextInterface, bizType, bizRef string) (*AnchorRecord, error) {
	key := anchorKey(bizType, bizRef)
	raw, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("anchor not found for key %s", key)
	}
	var record AnchorRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *SmartContract) VerifyDigest(ctx contractapi.TransactionContextInterface, bizType, bizRef, digest string) (bool, error) {
	record, err := s.QueryAnchor(ctx, bizType, bizRef)
	if err != nil {
		return false, err
	}
	return record.Digest == digest, nil
}

func anchorKey(bizType, bizRef string) string {
	return fmt.Sprintf("ANCHOR_%s_%s", bizType, bizRef)
}

func main() {
	cc, err := contractapi.NewChaincode(&SmartContract{})
	if err != nil {
		panic(err)
	}
	if err := cc.Start(); err != nil {
		panic(err)
	}
}
