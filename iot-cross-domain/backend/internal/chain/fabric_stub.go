package chain

import (
	"crypto/x509"
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type AnchorResult struct {
	TxHash      string `json:"txHash"`
	BlockHeight uint64 `json:"blockHeight"`
}

type Service interface {
	Anchor(bizType string, bizRef string, digest string) (AnchorResult, error)
}

type FabricGatewayService struct {
	clientConn *grpc.ClientConn
	gateway    *client.Gateway
	contract   *client.Contract
	anchorFunc string
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
	return &FabricGatewayService{
		clientConn: clientConn,
		gateway:    gw,
		contract:   contract,
		anchorFunc: cfg.AnchorFunction,
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
