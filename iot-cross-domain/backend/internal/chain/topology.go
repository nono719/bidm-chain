package chain

import (
	"net"
	"time"
)

type TopologyPeer struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Role     string `json:"role"`
	Online   bool   `json:"online"`
}

type TopologyOrg struct {
	MSPID       string         `json:"mspId"`
	DisplayName string         `json:"displayName"`
	DomainCode  string         `json:"domainCode"`
	Peers       []TopologyPeer `json:"peers"`
}

type TopologyOrderer struct {
	Name     string `json:"name"`
	MSPID    string `json:"mspId"`
	Endpoint string `json:"endpoint"`
	Online   bool   `json:"online"`
}

type Topology struct {
	Consortium string            `json:"consortium"`
	Channel    string            `json:"channel"`
	Chaincodes []string          `json:"chaincodes"`
	AnchorFunc string            `json:"anchorFunc"`
	Orgs       []TopologyOrg     `json:"orgs"`
	Orderers   []TopologyOrderer `json:"orderers"`
}

func DefaultTopology(channel string, chaincode string, anchorFunc string) Topology {
	return Topology{
		Consortium: "SampleConsortium",
		Channel:    channel,
		Chaincodes: []string{chaincode},
		AnchorFunc: anchorFunc,
		Orgs: []TopologyOrg{
			{
				MSPID:       "Org1MSP",
				DisplayName: "制造商域 Org1",
				DomainCode:  "domain-a",
				Peers: []TopologyPeer{
					{Name: "peer0.org1.example.com", Endpoint: "localhost:7051", Role: "endorser+committer"},
				},
			},
			{
				MSPID:       "Org2MSP",
				DisplayName: "服务商域 Org2",
				DomainCode:  "domain-b",
				Peers: []TopologyPeer{
					{Name: "peer0.org2.example.com", Endpoint: "localhost:9051", Role: "endorser+committer"},
				},
			},
		},
		Orderers: []TopologyOrderer{
			{Name: "orderer.example.com", MSPID: "OrdererMSP", Endpoint: "localhost:7050"},
		},
	}
}

func ProbeNode(endpoint string) bool {
	conn, err := net.DialTimeout("tcp", endpoint, 800*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func ProbeTopology(t *Topology) {
	for i := range t.Orgs {
		for j := range t.Orgs[i].Peers {
			t.Orgs[i].Peers[j].Online = ProbeNode(t.Orgs[i].Peers[j].Endpoint)
		}
	}
	for i := range t.Orderers {
		t.Orderers[i].Online = ProbeNode(t.Orderers[i].Endpoint)
	}
}
