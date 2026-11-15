package wasm

import "testing"

func TestDefaultLimits(t *testing.T) {
	if err := DefaultLimits().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLimitsReject(t *testing.T) {
	if (Limits{}).Validate() == nil {
		t.Fatal("empty invalid")
	}
}

func TestColdStartBudgetMs(t *testing.T) {
	if ColdStartBudgetMs() != 5 {
		t.Fatal("budget")
	}
}
