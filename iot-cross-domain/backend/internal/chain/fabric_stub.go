package chain

import (
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"iot-cross-domain/backend/internal/config"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	commonpb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	peerpb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

type AnchorResult struct {
	TxHash      string `json:"txHash"`
	BlockHeight uint64 `json:"blockHeight"`
}

type ChainInfo struct {
	Height            uint64 `json:"height"`
	CurrentBlockHash  string `json:"currentBlockHash"`
	PreviousBlockHash string `json:"previousBlockHash"`
}

type BlockSummary struct {
	Number       uint64 `json:"number"`
	DataHash     string `json:"dataHash"`
	PreviousHash string `json:"previousHash"`
	TxCount      int    `json:"txCount"`
}

type TxDetail struct {
	TxID              string `json:"txId"`
	ValidationCode    int32  `json:"validationCode"`
	ValidationMessage string `json:"validationMessage"`
	Valid             bool   `json:"valid"`
	BlockHeight       uint64 `json:"blockHeight,omitempty"`
}

type Service interface {
	Anchor(bizType string, bizRef string, digest string) (AnchorResult, error)
	ChainInfo() (*ChainInfo, error)
	BlockByNumber(num uint64) (*BlockSummary, error)
	TransactionByID(txID string) (*TxDetail, error)
	ChannelName() string
}

type FabricGatewayService struct {
	clientConn  *grpc.ClientConn
	gateway     *client.Gateway
	contract    *client.Contract
	qscc        *client.Contract
	channelName string
	anchorFunc  string
}

func NewFabricGatewayService(cfg config.FabricConfig) (*FabricGatewayService, error) {
	if cfg.CertPath == "" || cfg.KeyPath == "" {
		return nil, errors.New("missing FABRIC_CERT_PATH or FABRIC_KEY_PATH")
	}
	if cfg.UseTLS && cfg.TLSCertPath == "" {
		return nil, errors.New("FABRIC_USE_TLS=true requires FABRIC_TLS_CERT_PATH")
	}

	certPath, err := resolveCertPath(cfg.CertPath)
	if err != nil {
		return nil, err
	}
	keyPath, err := resolveKeyPath(cfg.KeyPath)
	if err != nil {
		return nil, err
	}

	id, err := newIdentity(cfg.MSPID, certPath)
	if err != nil {
		return nil, err
	}
	sign, err := newSign(keyPath)
	if err != nil {
		return nil, err
	}

	clientConn, err := newGRPCConnection(cfg)
	if err != nil {
		return nil, err
	}

	commitTimeout, err := time.ParseDuration(cfg.ConnectionTimeout)
	if err != nil {
		commitTimeout = 8 * time.Second
	}

	gw, err := client.Connect(
		id,
		client.WithSign(sign),
		client.WithClientConnection(clientConn),
		client.WithEvaluateTimeout(5*time.Second),
		client.WithEndorseTimeout(15*time.Second),
		client.WithSubmitTimeout(5*time.Second),
		client.WithCommitStatusTimeout(commitTimeout),
	)
	if err != nil {
		_ = clientConn.Close()
		return nil, err
	}

	network := gw.GetNetwork(cfg.ChannelName)
	contract := network.GetContract(cfg.ChaincodeName)
	qscc := network.GetContract("qscc")
	return &FabricGatewayService{
		clientConn:  clientConn,
		gateway:     gw,
		contract:    contract,
		qscc:        qscc,
		channelName: cfg.ChannelName,
		anchorFunc:  cfg.AnchorFunction,
	}, nil
}

func resolveCertPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return path, nil
	}
	matches, err := filepath.Glob(filepath.Join(path, "*.pem"))
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("no cert pem found in %s", path)
	}
	return matches[0], nil
}

func resolveKeyPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return path, nil
	}

	candidates, err := filepath.Glob(filepath.Join(path, "*"))
	if err != nil {
		return "", err
	}
	for _, candidate := range candidates {
		if strings.HasSuffix(candidate, "_sk") || strings.HasSuffix(candidate, ".pem") {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no private key file found in %s", path)
}

func (s *FabricGatewayService) Anchor(bizType string, bizRef string, digest string) (AnchorResult, error) {
	_, commit, err := s.contract.SubmitAsync(
		s.anchorFunc,
		client.WithArguments(bizType, bizRef, digest, fmt.Sprintf("%d", time.Now().Unix())),
	)
	if err != nil {
		return AnchorResult{}, err
	}

	status, err := commit.Status()
	if err != nil {
		return AnchorResult{}, err
	}
	if !status.Successful {
		return AnchorResult{}, fmt.Errorf("fabric commit failed: code=%v tx=%s", status.Code, commit.TransactionID())
	}

	return AnchorResult{
		TxHash:      commit.TransactionID(),
		BlockHeight: status.BlockNumber,
	}, nil
}

func (s *FabricGatewayService) ChannelName() string {
	return s.channelName
}

func (s *FabricGatewayService) ChainInfo() (*ChainInfo, error) {
	if s.qscc == nil {
		return nil, errors.New("qscc not initialized")
	}
	result, err := s.qscc.EvaluateTransaction("GetChainInfo", s.channelName)
	if err != nil {
		return nil, err
	}
	info := &commonpb.BlockchainInfo{}
	if err := proto.Unmarshal(result, info); err != nil {
		return nil, fmt.Errorf("decode BlockchainInfo: %w", err)
	}
	return &ChainInfo{
		Height:            info.Height,
		CurrentBlockHash:  hex.EncodeToString(info.CurrentBlockHash),
		PreviousBlockHash: hex.EncodeToString(info.PreviousBlockHash),
	}, nil
}

func (s *FabricGatewayService) BlockByNumber(num uint64) (*BlockSummary, error) {
	if s.qscc == nil {
		return nil, errors.New("qscc not initialized")
	}
	result, err := s.qscc.EvaluateTransaction("GetBlockByNumber", s.channelName, fmt.Sprintf("%d", num))
	if err != nil {
		return nil, err
	}
	block := &commonpb.Block{}
	if err := proto.Unmarshal(result, block); err != nil {
		return nil, fmt.Errorf("decode Block: %w", err)
	}
	out := &BlockSummary{}
	if block.Header != nil {
		out.Number = block.Header.Number
		out.DataHash = hex.EncodeToString(block.Header.DataHash)
		out.PreviousHash = hex.EncodeToString(block.Header.PreviousHash)
	}
	if block.Data != nil {
		out.TxCount = len(block.Data.Data)
	}
	return out, nil
}

func (s *FabricGatewayService) TransactionByID(txID string) (*TxDetail, error) {
	if s.qscc == nil {
		return nil, errors.New("qscc not initialized")
	}
	result, err := s.qscc.EvaluateTransaction("GetTransactionByID", s.channelName, txID)
	if err != nil {
		return nil, err
	}
	pt := &peerpb.ProcessedTransaction{}
	if err := proto.Unmarshal(result, pt); err != nil {
		return nil, fmt.Errorf("decode ProcessedTransaction: %w", err)
	}
	code := pt.ValidationCode
	name := peerpb.TxValidationCode_name[code]
	if name == "" {
		name = fmt.Sprintf("UNKNOWN(%d)", code)
	}
	return &TxDetail{
		TxID:              txID,
		ValidationCode:    code,
		ValidationMessage: name,
		Valid:             code == int32(peerpb.TxValidationCode_VALID),
	}, nil
}

func (s *FabricGatewayService) Close() error {
	if s.gateway != nil {
		s.gateway.Close()
	}
	if s.clientConn != nil {
		return s.clientConn.Close()
	}
	return nil
}

func newIdentity(mspID string, certPath string) (*identity.X509Identity, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	certificate, err := identity.CertificateFromPEM(certPEM)
	if err != nil {
		return nil, err
	}
	return identity.NewX509Identity(mspID, certificate)
}

func newSign(keyPath string) (identity.Sign, error) {
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("invalid private key PEM")
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		privateKey, err = x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
	}
	return identity.NewPrivateKeySign(privateKey)
}

func newGRPCConnection(cfg config.FabricConfig) (*grpc.ClientConn, error) {
	if cfg.UseTLS {
		transportCredentials, err := credentials.NewClientTLSFromFile(cfg.TLSCertPath, cfg.PeerHostAlias)
		if err != nil {
			return nil, err
		}
		return grpc.Dial(cfg.PeerEndpoint, grpc.WithTransportCredentials(transportCredentials))
	}
	return grpc.Dial(cfg.PeerEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
}
