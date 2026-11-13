package db

import "testing"

func TestPreviewHostname(t *testing.T) {
	if got := PreviewHostname("Blog Demo", 42); got != "pr-42.blog-demo.preview.localhost" {
		t.Fatalf("got %s", got)
	}
	if got := PreviewHostname("app", 7); got != "pr-7.app.preview.localhost" {
		t.Fatalf("got %s", got)
	}
}
