package envutil

import (
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	t.Setenv("K", "v")
	if got := Get("K", "d"); got != "v" {
		t.Fatalf("Get=%q", got)
	}

	if got := Get("NO", "d"); got != "d" {
		t.Fatalf("Get default=%q", got)
	}
}

func TestGetInt(t *testing.T) {
	t.Setenv("I", "12")
	if got := GetInt("I", 3); got != 12 {
		t.Fatalf("GetInt=%d", got)
	}

	t.Setenv("I", "xx")
	if got := GetInt("I", 3); got != 3 {
		t.Fatalf("GetInt default=%d", got)
	}
}

func TestGetFloat64(t *testing.T) {
	t.Setenv("F", "12.5")
	if got := GetFloat64("F", 3.0); got != 12.5 {
		t.Fatalf("GetFloat64=%f", got)
	}

	t.Setenv("F", "xx")
	if got := GetFloat64("F", 3.0); got != 3.0 {
		t.Fatalf("GetFloat64 default=%f", got)
	}

	t.Setenv("F", "NaN")
	if got := GetFloat64("F", 3.0); got != 3.0 {
		t.Fatalf("GetFloat64 NaN default=%f", got)
	}

	t.Setenv("F", "+Inf")
	if got := GetFloat64("F", 3.0); got != 3.0 {
		t.Fatalf("GetFloat64 +Inf default=%f", got)
	}
}

func TestGetDuration(t *testing.T) {
	t.Setenv("D", "2s")
	if got := GetDuration("D", time.Second); got != 2*time.Second {
		t.Fatalf("GetDuration=%s", got)
	}

	t.Setenv("D", "0s")
	if got := GetDuration("D", time.Second); got != time.Second {
		t.Fatalf("GetDuration default=%s", got)
	}
}

func TestGetBool(t *testing.T) {
	t.Setenv("B", "yes")
	if !GetBool("B", false) {
		t.Fatal("expected true")
	}

	t.Setenv("B", "off")
	if GetBool("B", true) {
		t.Fatal("expected false")
	}
}
