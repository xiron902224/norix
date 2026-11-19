package api

import "testing"

func TestValidateComposePath(t *testing.T) {
	ok := map[string]string{
		"":                          "docker-compose.yml",
		"docker-compose.yml":        "docker-compose.yml",
		"./deploy/compose.yml":      "deploy/compose.yml",
		"infra/../compose.prod.yml": "compose.prod.yml",
	}
	for input, want := range ok {
		got, err := validateComposePath(input)
		if err != nil || got != want {
			t.Fatalf("validateComposePath(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, bad := range []string{"/etc/passwd", "../outside.yml", "a/../../x.yml", `deploy\compose.yml`} {
		if _, err := validateComposePath(bad); err == nil {
			t.Fatalf("validateComposePath(%q) must fail", bad)
		}
	}
}

func TestValidateStackName(t *testing.T) {
	for _, good := range []string{"infra", "api_v2", "Dexi-1", "rodo example", "Infra de abogados", "  todo.app  "} {
		if _, err := validateStackName(good); err != nil {
			t.Fatalf("validateStackName(%q): %v", good, err)
		}
	}
	if got, err := validateStackName("  rodo example  "); err != nil || got != "rodo example" {
		t.Fatalf("trimmed name = %q, %v", got, err)
	}
	for _, bad := range []string{"", "semi;colon", "x/y", "a\nb", "../x"} {
		if _, err := validateStackName(bad); err == nil {
			t.Fatalf("validateStackName(%q) must fail", bad)
		}
	}
}
