package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/omerfarukaydin/atlas/internal/tools"
)

const (
	defaultTimeout = 2 * time.Minute
	maxOutputBytes = 32 * 1024
)

type ExecTool struct{}

func (ExecTool) Name() string { return "run_shell" }
func (ExecTool) Description() string {
	return "Yerel makinede bir kabuk komutu çalıştırır ve birleştirilmiş stdout/stderr çıktısını döndürür."
}
func (ExecTool) RequiresApproval() bool { return true }

func (ExecTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "Çalıştırılacak kabuk komutu"}
		},
		"required": ["command"]
	}`)
}

func (ExecTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return tools.Result{Content: "geçersiz girdi: " + err.Error(), IsError: true}, nil
	}
	if args.Command == "" {
		return tools.Result{Content: "command alanı boş olamaz", IsError: true}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", args.Command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", args.Command)
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()
	output := out.String()
	truncated := false
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes]
		truncated = true
	}

	var summary string
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		summary = fmt.Sprintf("[zaman aşımı: %s]\n", defaultTimeout)
	case runErr != nil:
		summary = fmt.Sprintf("[çıkış kodu hatası: %v]\n", runErr)
	}
	if truncated {
		summary += "[çıktı kesildi]\n"
	}

	isError := runErr != nil
	return tools.Result{Content: summary + output, IsError: isError}, nil
}
