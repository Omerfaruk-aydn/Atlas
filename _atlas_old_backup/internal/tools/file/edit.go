package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/omerfarukaydin/atlas/internal/tools"
)

type EditTool struct{}

func (EditTool) Name() string { return "edit_file" }
func (EditTool) Description() string {
	return "Bir dosyada tam olarak eşleşen bir metin parçasını başka bir metinle değiştirir. old_string dosyada tam olarak bir kez geçmelidir, aksi halde replace_all kullanılmalıdır."
}
func (EditTool) RequiresApproval() bool { return true }

func (EditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Düzenlenecek dosyanın yolu"},
			"old_string": {"type": "string", "description": "Değiştirilecek tam metin"},
			"new_string": {"type": "string", "description": "Yerine yazılacak metin"},
			"replace_all": {"type": "boolean", "description": "Tüm eşleşmeleri değiştir (varsayılan: false)"}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}

type editArgs struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

// computeEdit reads the target file and applies the requested replacement
// in memory, without writing anything back. Shared by Preview and Execute
// so both agree on exactly what change would be made.
func computeEdit(args editArgs) (content, updated string, count int, err error) {
	data, err := os.ReadFile(args.Path)
	if err != nil {
		return "", "", 0, err
	}
	content = string(data)

	count = strings.Count(content, args.OldString)
	if count == 0 {
		return content, "", 0, fmt.Errorf("old_string dosyada bulunamadı: %s", args.Path)
	}
	if count > 1 && !args.ReplaceAll {
		return content, "", count, fmt.Errorf("old_string dosyada %d kez geçiyor; tek bir eşleşme için daha fazla bağlam ekle ya da replace_all=true kullan", count)
	}

	if args.ReplaceAll {
		updated = strings.ReplaceAll(content, args.OldString, args.NewString)
	} else {
		updated = strings.Replace(content, args.OldString, args.NewString, 1)
	}
	return content, updated, count, nil
}

// Preview computes the would-be result of the edit without writing it, so
// the approval UI can diff it against the current file content.
func (EditTool) Preview(input json.RawMessage) (tools.Preview, error) {
	var args editArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tools.Preview{}, err
	}
	content, updated, _, err := computeEdit(args)
	if err != nil {
		return tools.Preview{}, err
	}
	return tools.Preview{Path: args.Path, Old: content, New: updated}, nil
}

func (EditTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	var args editArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return tools.Result{Content: "geçersiz girdi: " + err.Error(), IsError: true}, nil
	}
	if args.Path == "" || args.OldString == "" {
		return tools.Result{Content: "path ve old_string alanları boş olamaz", IsError: true}, nil
	}
	if args.OldString == args.NewString {
		return tools.Result{Content: "old_string ve new_string aynı", IsError: true}, nil
	}

	_, updated, count, err := computeEdit(args)
	if err != nil {
		return tools.Result{Content: err.Error(), IsError: true}, nil
	}

	if err := os.WriteFile(args.Path, []byte(updated), 0o644); err != nil {
		return tools.Result{Content: err.Error(), IsError: true}, nil
	}

	replaced := 1
	if args.ReplaceAll {
		replaced = count
	}
	return tools.Result{Content: fmt.Sprintf("Düzenlendi: %s (%d değişiklik)", args.Path, replaced)}, nil
}
