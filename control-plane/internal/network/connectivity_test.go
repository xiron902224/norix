package network

import (
	"strings"
	"testing"
)

// Contract: workload FQDNs resolve to overlay addresses that appear in peer AllowedIPs (#48).
func TestWorkloadToWorkloadConnectivityContract(t *testing.T) {
	apiFQDN := ServiceFQDN("shop", "api")
	webFQDN := ServiceFQDN("shop", "web")
	if !strings.HasSuffix(apiFQDN, "."+DNSSuffix) || !strings.HasSuffix(webFQDN, "."+DNSSuffix) {
		t.Fatalf("fqdn suffix: %s %s", apiFQDN, webFQDN)
	}

	apiAddr := "10.88.0.2/32"
	webAddr := "10.88.0.3/32"
	hosts := RenderHostsFile([]DNSRecord{
		{Name: apiFQDN, Type: "A", Value: strings.TrimSuffix(apiAddr, "/32")},
		{Name: webFQDN, Type: "A", Value: strings.TrimSuffix(webAddr, "/32")},
	})
	if !strings.Contains(hosts, "10.88.0.2 "+apiFQDN) {
		t.Fatalf("hosts missing api: %s", hosts)
	}
	if !strings.Contains(hosts, "10.88.0.3 "+webFQDN) {
		t.Fatalf("hosts missing web: %s", hosts)
	}

	peers := []PeerConfig{{
		PublicKey:  "peerPub",
		AllowedIPs: apiAddr,
		Endpoint:   "worker-a:51820",
	}}
	cfg := RenderWGQuick("wg-norix", "priv", webAddr, DefaultWGPort, peers)
	if !strings.Contains(cfg, "AllowedIPs = "+apiAddr) {
		t.Fatalf("peer AllowedIPs missing api overlay addr:\n%s", cfg)
	}
	if !strings.Contains(cfg, "Address = "+webAddr) {
		t.Fatalf("local address missing:\n%s", cfg)
	}
}

func TestGenerateKeyPairDistinct(t *testing.T) {
	a, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if a.PublicKey == "" || a.PrivateKey == "" || a.PublicKey == b.PublicKey {
		t.Fatalf("keys not distinct: %+v %+v", a, b)
	}
}
