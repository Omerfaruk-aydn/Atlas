package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/omerfarukaydin/atlas/internal/tools"
)

type WriteTool struct{}

func (WriteTool) Name() string { return "write_file" }
func (WriteTool) Description() string {
	return "Bir dosyayı verilen içerikle oluşturur veya tamamen üzerine yazar. Var olan bir dosyanın küçük bir kısmını değiştirmek için edit_file kullan."
}
func (WriteTool) RequiresApproval() bool { return true }

func (WriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Yazılacak dosyanın yolu"},
			"content": {"type": "string", "description": "Dosyaya yazılacak tam içerik"}
		},
		"required": ["path", "content"]
	}`)
}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Preview reads the file's current content (empty if it doesn't exist yet)
// so the caller can diff it against the proposed content before approving.
func (WriteTool) Preview(input json.RawMessage) (tools.Preview, error) {
	var args writeArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tools.Preview{}, err
	}
	old, _ := os.ReadFile(args.Path) // nil/empty if the file doesn't exist yet
	return tools.Preview{Path: args.Path, Old: string(old), New: args.Content}, nil
}

func (WriteTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	var args writeArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tools.Result{Content: "geçersiz girdi: " + err.Error(), IsError: true}, nil
	}
	if args.Path == "" {
		return tools.Result{Content: "path alanı boş olamaz", IsError: true}, nil
	}

	if dir := filepath.Dir(args.Path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return tools.Result{Content: err.Error(), IsError: true}, nil
		}
	}

	if err := os.WriteFile(args.Path, []byte(args.Content), 0o644); err != nil {
		return tools.Result{Content: err.Error(), IsError: true}, nil
	}

	return tools.Result{Content: fmt.Sprintf("Yazıldı: %s (%d bayt)", args.Path, len(args.Content))}, nil
}
