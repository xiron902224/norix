package db

import "testing"

func TestWasmListenPortStable(t *testing.T) {
	a := WasmListenPort("11111111-1111-1111-1111-111111111111")
	b := WasmListenPort("11111111-1111-1111-1111-111111111111")
	if a != b || a < 19000 || a > 19999 {
		t.Fatalf("port=%d", a)
	}
	c := WasmListenPort("22222222-2222-2222-2222-222222222222")
	if c == a {
		t.Fatal("expected different ids to differ usually")
	}
}
