package api

import (
	"net/http"
	"testing"
)

func TestBootstrapTransportAllowed(t *testing.T) {
	t.Setenv(AllowInsecureBootstrapEnv, "")

	tests := []struct {
		name     string
		remote   string
		header   http.Header
		tls      bool
		insecure string
		want     bool
	}{
		{name: "loopback ipv4", remote: "127.0.0.1:54321", want: true},
		{name: "loopback ipv6", remote: "[::1]:54321", want: true},
		{name: "public http", remote: "203.0.113.9:44321", want: false},
		{name: "docker bridge http", remote: "172.17.0.1:45000", want: false},
		{
			name:   "loopback proxy https",
			remote: "127.0.0.1:4443",
			header: http.Header{"X-Forwarded-Proto": []string{"https"}},
			want:   true,
		},
		{
			name:   "spoofed xfp from public",
			remote: "203.0.113.9:1",
			header: http.Header{"X-Forwarded-Proto": []string{"https"}},
			want:   false,
		},
		{
			name:     "explicit opt-in",
			remote:   "203.0.113.9:1",
			insecure: "1",
			want:     true,
		},
		{name: "direct tls", remote: "203.0.113.9:1", tls: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(AllowInsecureBootstrapEnv, tt.insecure)
			header := tt.header
			if header == nil {
				header = http.Header{}
			}
			got := BootstrapTransportAllowed(tt.remote, header, tt.tls)
			if got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestPeerIP(t *testing.T) {
	if got := peerIP("127.0.0.1:9"); got == nil || !got.IsLoopback() {
		t.Fatalf("peerIP loopback = %v", got)
	}
	if got := peerIP("[::1]:9"); got == nil || !got.IsLoopback() {
		t.Fatalf("peerIP v6 = %v", got)
	}
}
