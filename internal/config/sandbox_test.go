package config

import "testing"

func TestSandboxMaxProcessesOrDefaultUsesTheDefaultWhenUnset(t *testing.T) {
	var s Sandbox
	if got := s.MaxProcessesOrDefault(); got != defaultSandboxMaxProcesses {
		t.Fatalf("MaxProcessesOrDefault() = %d, want %d", got, defaultSandboxMaxProcesses)
	}
}

func TestSandboxMaxProcessesOrDefaultHonorsAPositiveValue(t *testing.T) {
	s := Sandbox{MaxProcesses: 12}
	if got := s.MaxProcessesOrDefault(); got != 12 {
		t.Fatalf("MaxProcessesOrDefault() = %d, want 12", got)
	}
}

func TestSandboxMaxProcessesOrDefaultRejectsANonPositiveValue(t *testing.T) {
	s := Sandbox{MaxProcesses: -5}
	if got := s.MaxProcessesOrDefault(); got != defaultSandboxMaxProcesses {
		t.Fatalf("MaxProcessesOrDefault() = %d, want %d for a negative configured value", got, defaultSandboxMaxProcesses)
	}
}

func TestSandboxMaxMemoryBytesIsZeroWhenUnset(t *testing.T) {
	var s Sandbox
	if got := s.MaxMemoryBytes(); got != 0 {
		t.Fatalf("MaxMemoryBytes() = %d, want 0", got)
	}
}

func TestSandboxMaxMemoryBytesConvertsMegabytes(t *testing.T) {
	s := Sandbox{MaxMemoryMB: 2}
	if got, want := s.MaxMemoryBytes(), uint64(2*1024*1024); got != want {
		t.Fatalf("MaxMemoryBytes() = %d, want %d", got, want)
	}
}
