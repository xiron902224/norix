package db

import "testing"

func TestPgDumpArgsIncludeClean(t *testing.T) {
	args := pgDumpArgs("postgres://norix@127.0.0.1/norix", "/tmp/norix.sql")
	joined := " " + joinArgs(args) + " "
	for _, want := range []string{" --clean ", " --if-exists ", " --no-owner ", " --file=/tmp/norix.sql "} {
		if !contains(joined, want) {
			t.Fatalf("dump args missing %q in %q", want, joined)
		}
	}
}

func TestPublicSchemaResetDropsBeforeCreate(t *testing.T) {
	sql := publicSchemaResetSQL()
	drop := indexOf(sql, "DROP SCHEMA IF EXISTS public CASCADE")
	create := indexOf(sql, "CREATE SCHEMA public")
	if drop < 0 || create < 0 || drop > create {
		t.Fatalf("reset SQL must drop public before recreating it: %s", sql)
	}
	if indexOf(sql, "pg_terminate_backend") < 0 {
		t.Fatalf("reset SQL must disconnect other sessions so DROP SCHEMA can take the lock")
	}
}

func joinArgs(args []string) string {
	out := ""
	for _, arg := range args {
		if out != "" {
			out += " "
		}
		out += arg
	}
	return out
}

func contains(haystack, needle string) bool {
	return indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
