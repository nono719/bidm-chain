package config

import (
	"os"
)

type Config struct {
	Port         string
	MySQLDSN     string
	JWTSecret    string
	OracleSecret string
	Fabric       FabricConfig
}

type FabricConfig struct {
	MSPID             string
	CertPath          string
	KeyPath           string
	TLSCertPath       string
	PeerEndpoint      string
	PeerHostAlias     string
	ChannelName       string
	ChaincodeName     string
	AnchorFunction    string
	UseTLS            bool
	ConnectionTimeout string
}

func Load() Config {
	return Config{
		Port:         getEnv("APP_PORT", "8080"),
		MySQLDSN:     getEnv("MYSQL_DSN", "root:root@tcp(127.0.0.1:3306)/iot_auth?charset=utf8mb4&parseTime=True&loc=Local"),
		JWTSecret:    getEnv("JWT_SECRET", "change-this-secret"),
		OracleSecret: getEnv("ORACLE_SECRET", "oracle-local-secret"),
		Fabric: FabricConfig{
			MSPID:             getEnv("FABRIC_MSP_ID", "Org1MSP"),
			CertPath:          getEnv("FABRIC_CERT_PATH", ""),
			KeyPath:           getEnv("FABRIC_KEY_PATH", ""),
			TLSCertPath:       getEnv("FABRIC_TLS_CERT_PATH", ""),
			PeerEndpoint:      getEnv("FABRIC_PEER_ENDPOINT", "localhost:7051"),
			PeerHostAlias:     getEnv("FABRIC_PEER_HOST_ALIAS", "peer0.org1.example.com"),
			ChannelName:       getEnv("FABRIC_CHANNEL", "mychannel"),
			ChaincodeName:     getEnv("FABRIC_CHAINCODE", "basic"),
			AnchorFunction:    getEnv("FABRIC_ANCHOR_FUNCTION", "AnchorRecord"),
			UseTLS:            getEnv("FABRIC_USE_TLS", "true") != "false",
			ConnectionTimeout: getEnv("FABRIC_CONN_TIMEOUT", "8s"),
		},
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
