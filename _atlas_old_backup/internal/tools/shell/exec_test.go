package shell

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestExecToolRunsCommand(t *testing.T) {
	cmd := "echo hello-atlas"
	input, _ := json.Marshal(map[string]string{"command": cmd})

	res, err := ExecTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "hello-atlas") {
		t.Errorf("expected output to contain 'hello-atlas', got %q", res.Content)
	}
}

func TestExecToolReportsNonZeroExit(t *testing.T) {
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "exit 3"
	} else {
		cmd = "exit 3"
	}
	input, _ := json.Marshal(map[string]string{"command": cmd})

	res, err := ExecTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError for a non-zero exit code")
	}
}

func TestExecToolMissingCommand(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"command": ""})
	res, err := ExecTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError for an empty command")
	}
}
