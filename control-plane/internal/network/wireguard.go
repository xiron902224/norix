package network

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/curve25519"
)

const (
	OverlayCIDR   = "10.88.0.0/16"
	DNSSuffix     = "norix.internal"
	DefaultWGPort = 51820
	DefaultMTU    = 1420
)

type KeyPair struct {
	PrivateKey string
	PublicKey  string
}

func GenerateKeyPair() (KeyPair, error) {
	var private [32]byte
	if _, err := rand.Read(private[:]); err != nil {
		return KeyPair{}, err
	}
	// Clamp as WireGuard expects.
	private[0] &= 248
	private[31] &= 127
	private[31] |= 64
	var public [32]byte
	curve25519.ScalarBaseMult(&public, &private)
	return KeyPair{
		PrivateKey: base64.StdEncoding.EncodeToString(private[:]),
		PublicKey:  base64.StdEncoding.EncodeToString(public[:]),
	}, nil
}

func NodeAddress(index uint32) string {
	if index == 0 {
		index = 1
	}
	hi := (index >> 8) & 0xff
	lo := index & 0xff
	return fmt.Sprintf("10.88.%d.%d/32", hi, lo)
}

func ServiceFQDN(projectName, workloadName string) string {
	return fmt.Sprintf("%s.%s.%s", sanitizeDNS(workloadName), sanitizeDNS(projectName), DNSSuffix)
}

// DNSRecord is an internal overlay DNS entry (hosts-file style).
type DNSRecord struct {
	Name  string
	Type  string // A | AAAA | CNAME
	Value string
}

// RenderHostsFile builds a hosts snapshot for service discovery on the overlay.
func RenderHostsFile(records []DNSRecord) string {
	var b strings.Builder
	b.WriteString("# norix internal DNS snapshot\n")
	for _, record := range records {
		typ := strings.ToUpper(strings.TrimSpace(record.Type))
		if typ != "A" && typ != "AAAA" {
			continue
		}
		name := strings.TrimSpace(record.Name)
		value := strings.TrimSpace(record.Value)
		if name == "" || value == "" {
			continue
		}
		fmt.Fprintf(&b, "%s %s\n", value, name)
	}
	return b.String()
}

func ProjectMark(projectIndex int) int {
	if projectIndex <= 0 {
		return 100
	}
	return 100 + projectIndex
}

func RenderWGQuick(interfaceName, privateKey, address string, listenPort int, peers []PeerConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Interface]\nPrivateKey = %s\nAddress = %s\nListenPort = %d\nMTU = %d\n", privateKey, address, listenPort, DefaultMTU)
	for _, peer := range peers {
		fmt.Fprintf(&b, "\n[Peer]\nPublicKey = %s\nAllowedIPs = %s\nPersistentKeepalive = 25\n", peer.PublicKey, peer.AllowedIPs)
		if peer.Endpoint != "" {
			fmt.Fprintf(&b, "Endpoint = %s\n", peer.Endpoint)
		}
	}
	return b.String()
}

type PeerConfig struct {
	PublicKey  string
	AllowedIPs string
	Endpoint   string
	NodeID     string
	NodeName   string
	Address    string
}

func sanitizeDNS(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			b.WriteRune(ch)
		} else if ch == '_' || ch == '.' || ch == ' ' {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "app"
	}
	return out
}
