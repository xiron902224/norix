package main

import "strings"

func normalizeAPIBase(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func connectProcedurePath(procedure string) string {
	procedure = strings.TrimSpace(procedure)
	if !strings.HasPrefix(procedure, "/") {
		procedure = "/" + procedure
	}
	return "/api" + procedure
}
