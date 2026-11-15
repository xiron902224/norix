package main

import "testing"

func TestNormalizeAPIBase(t *testing.T) {
	if normalizeAPIBase("http://127.0.0.1:8080/") != "http://127.0.0.1:8080" {
		t.Fatal("trim slash")
	}
}

func TestConnectProcedurePath(t *testing.T) {
	if connectProcedurePath("v1.ProjectService/ListProjects") != "/api/v1.ProjectService/ListProjects" {
		t.Fatal(connectProcedurePath("v1.ProjectService/ListProjects"))
	}
}
