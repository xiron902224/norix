package db

import "testing"

func TestFilterLogSegmentsContains(t *testing.T) {
	segments := []LogSegment{
		{ID: 1, Body: "Server Started at port 3000"},
		{ID: 2, Body: "Connected to Redis"},
		{ID: 3, Body: "ECONNREFUSED 172.17.0.1:6379"},
	}
	got := FilterLogSegmentsContains(segments, "redis")
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("want redis hit, got %+v", got)
	}
	got = FilterLogSegmentsContains(segments, "")
	if len(got) != 3 {
		t.Fatalf("empty query keeps all, got %d", len(got))
	}
	got = FilterLogSegmentsContains(segments, "missing-token")
	if len(got) != 0 {
		t.Fatalf("want empty, got %d", len(got))
	}
}
