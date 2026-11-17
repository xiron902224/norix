package db

import "testing"

func TestValidateVolumeMounts(t *testing.T) {
	if err := ValidateVolumeMounts([]VolumeMount{{Name: "data", MountPath: "/data"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateVolumeMounts([]VolumeMount{{Name: "../x", MountPath: "/data"}}); err == nil {
		t.Fatal("expected invalid name")
	}
	if err := ValidateVolumeMounts([]VolumeMount{{Name: "data", MountPath: "relative"}}); err == nil {
		t.Fatal("expected absolute path")
	}
	if err := ValidateVolumeMounts([]VolumeMount{{Name: "data", MountPath: "/data/../etc"}}); err == nil {
		t.Fatal("expected unclean path")
	}
	if err := ValidateVolumeMounts([]VolumeMount{
		{Name: "a", MountPath: "/data"},
		{Name: "a", MountPath: "/other"},
	}); err == nil {
		t.Fatal("expected duplicate name")
	}
}

func TestDockerVolumeName(t *testing.T) {
	got := DockerVolumeName("abcdef12-3456-7890-abcd-ef1234567890", "My Data!")
	if !hasPrefix(got, "norix-wv-abcdef123456-") {
		t.Fatalf("got %q", got)
	}
}

func TestParseVolumeOpMessage(t *testing.T) {
	cases := []struct {
		msg            string
		wantID, wantSt string
		wantOK         bool
	}{
		{"norix-volume-op:abc123:done:/var/lib/norix/volume-backups/x.tgz", "abc123", "done", true},
		{"running; norix-volume-op:deadbeef:failed:boom", "deadbeef", "failed", true},
		{"norix-volume-op:x:done:path; extra", "x", "done", true},
		{"plain status", "", "", false},
		{"norix-volume-op::done:x", "", "", false},
	}
	for _, tc := range cases {
		id, st, detail, ok := parseVolumeOpMessage(tc.msg)
		if ok != tc.wantOK {
			t.Fatalf("%q ok=%v want %v", tc.msg, ok, tc.wantOK)
		}
		if !tc.wantOK {
			continue
		}
		if id != tc.wantID || st != tc.wantSt {
			t.Fatalf("%q => id=%q st=%q detail=%q", tc.msg, id, st, detail)
		}
	}
}

func hasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
