package main

import (
	"reflect"
	"testing"
)

func TestAptArgv_Root_SkipsSudo(t *testing.T) {
	got := aptArgv(0, "update", "-qq")
	want := []string{"apt-get", "update", "-qq"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("aptArgv(0) = %v, want %v", got, want)
	}
}

func TestAptArgv_NonRoot_UsesSudo(t *testing.T) {
	got := aptArgv(1000, "install", "-y", "curl", "jq")
	want := []string{"sudo", "apt-get", "install", "-y", "curl", "jq"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("aptArgv(1000) = %v, want %v", got, want)
	}
}

func TestAptArgv_EmptyExtra(t *testing.T) {
	got := aptArgv(0, "update")
	want := []string{"apt-get", "update"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("aptArgv no extra = %v, want %v", got, want)
	}
}

func TestAptArgv_NegativeEuid_TreatsAsNonRoot(t *testing.T) {
	// os.Geteuid() returns -1 on Windows; ensure the non-root branch runs.
	got := aptArgv(-1, "install", "-y", "curl")
	if got[0] != "sudo" {
		t.Errorf("aptArgv(-1)[0] = %q, want sudo", got[0])
	}
}
