package chain

import (
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"

	commonpb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	msppb "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	peerpb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/protobuf/proto"
)

type EndorserInfo struct {
	MSPID      string `json:"mspId"`
	CommonName string `json:"commonName"`
	Issuer     string `json:"issuer"`
	Serial     string `json:"serial"`
	SigShort   string `json:"sigShort"`
}

type TxParseResult struct {
	ChannelID   string         `json:"channelId"`
	HeaderType  string         `json:"headerType"`
	TxID        string         `json:"txId"`
	TimestampMs int64          `json:"timestampMs"`
	Creator     *EndorserInfo  `json:"creator,omitempty"`
	Endorsers   []EndorserInfo `json:"endorsers"`
}

func parseEndorsements(envelope *commonpb.Envelope) (*TxParseResult, error) {
	if envelope == nil {
		return nil, errors.New("nil envelope")
	}
	payload := &commonpb.Payload{}
	if err := proto.Unmarshal(envelope.Payload, payload); err != nil {
		return nil, fmt.Errorf("payload: %w", err)
	}
	out := &TxParseResult{}

	if payload.Header != nil {
		ch := &commonpb.ChannelHeader{}
		if err := proto.Unmarshal(payload.Header.ChannelHeader, ch); err == nil {
			out.ChannelID = ch.ChannelId
			out.TxID = ch.TxId
			out.HeaderType = commonpb.HeaderType_name[ch.Type]
			if ch.Timestamp != nil {
				out.TimestampMs = ch.Timestamp.Seconds*1000 + int64(ch.Timestamp.Nanos)/1e6
			}
		}
		sigHdr := &commonpb.SignatureHeader{}
		if err := proto.Unmarshal(payload.Header.SignatureHeader, sigHdr); err == nil {
			if creator, err := decodeIdentity(sigHdr.Creator, nil); err == nil {
				out.Creator = creator
			}
		}
	}

	tx := &peerpb.Transaction{}
	if err := proto.Unmarshal(payload.Data, tx); err != nil {
		return out, nil
	}
	for _, action := range tx.Actions {
		ccap := &peerpb.ChaincodeActionPayload{}
		if err := proto.Unmarshal(action.Payload, ccap); err != nil {
			continue
		}
		if ccap.Action == nil {
			continue
		}
		for _, end := range ccap.Action.Endorsements {
			info, err := decodeIdentity(end.Endorser, end.Signature)
			if err != nil {
				continue
			}
			out.Endorsers = append(out.Endorsers, *info)
		}
	}
	return out, nil
}

func decodeIdentity(serialized []byte, signature []byte) (*EndorserInfo, error) {
	si := &msppb.SerializedIdentity{}
	if err := proto.Unmarshal(serialized, si); err != nil {
		return nil, err
	}
	info := &EndorserInfo{MSPID: si.Mspid}
	if len(signature) > 0 {
		hexSig := hex.EncodeToString(signature)
		if len(hexSig) > 24 {
			info.SigShort = hexSig[:12] + "..." + hexSig[len(hexSig)-8:]
		} else {
			info.SigShort = hexSig
		}
	}
	block, _ := pem.Decode(si.IdBytes)
	if block == nil {
		return info, nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return info, nil
	}
	info.CommonName = cert.Subject.CommonName
	info.Issuer = cert.Issuer.CommonName
	info.Serial = cert.SerialNumber.String()
	return info, nil
}
