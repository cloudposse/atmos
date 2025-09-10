package failing_tests

import "testing"

func TestFail1(t *testing.T) {
	t.Fatal("This test fails intentionally")
}

func TestFail2(t *testing.T) {
	t.Error("This test also fails")
	t.Error("With multiple errors")
}

func TestFailWithMessage(t *testing.T) {
	if 2+2 != 5 {
		t.Fatalf("Math is working correctly, but we want it to fail: %d != %d", 4, 5)
	}
}