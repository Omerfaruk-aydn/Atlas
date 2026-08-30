package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/omerfarukaydin/atlas/internal/tools"
)

const maxReadBytes = 256 * 1024

type ReadTool struct{}

func (ReadTool) Name() string           { return "read_file" }
func (ReadTool) Description() string    { return "Bir dosyanın içeriğini okur ve döndürür." }
func (ReadTool) RequiresApproval() bool { return false }

func (ReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Okunacak dosyanın yolu"}
		},
		"required": ["path"]
	}`)
}

func (ReadTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return tools.Result{Content: "geçersiz girdi: " + err.Error(), IsError: true}, nil
	}
	if args.Path == "" {
		return tools.Result{Content: "path alanı boş olamaz", IsError: true}, nil
	}

	info, err := os.Stat(args.Path)
	if err != nil {
		return tools.Result{Content: err.Error(), IsError: true}, nil
	}
	if info.IsDir() {
		return tools.Result{Content: args.Path + " bir dizin, dosya değil", IsError: true}, nil
	}

	data, err := os.ReadFile(args.Path)
	if err != nil {
		return tools.Result{Content: err.Error(), IsError: true}, nil
	}
	if len(data) > maxReadBytes {
		data = data[:maxReadBytes]
		return tools.Result{Content: fmt.Sprintf("%s\n\n[...%d byte sonra kesildi]", data, info.Size()-maxReadBytes)}, nil
	}

	return tools.Result{Content: string(data)}, nil
}
