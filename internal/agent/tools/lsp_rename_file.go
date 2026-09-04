package tools

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-powernap/pkg/lsp/protocol"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/filetracker"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/history"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/lsp"
	lsputil "github.com/Omerfaruk-aydn/Atlas-Agent/internal/lsp/util"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
)

const LspRenameFileToolName = "lsp_rename_file"

//go:embed lsp_rename_file.md
var lspRenameFileDescription string

type LspRenameFileParams struct {
	OldPath string `json:"old_path" description:"The file's current path."`
	NewPath string `json:"new_path" description:"Where it should end up. Must not already exist."`
}

func NewLspRenameFileTool(
	lspManager *lsp.Manager,
	permissions permission.Service,
	files history.Service,
	filetracker filetracker.Service,
	workingDir string,
) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		LspRenameFileToolName,
		lspRenameFileDescription,
		func(ctx context.Context, params LspRenameFileParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.OldPath == "" {
				return fantasy.NewTextErrorResponse("old_path is required"), nil
			}
			if params.NewPath == "" {
				return fantasy.NewTextErrorResponse("new_path is required"), nil
			}

			oldPath := resolvePath(workingDir, params.OldPath)
			newPath := resolvePath(workingDir, params.NewPath)

			if _, err := os.Stat(oldPath); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("%s does not exist", relOrAbs(oldPath, workingDir))), nil
			}
			if _, err := os.Stat(newPath); err == nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("%s already exists", relOrAbs(newPath, workingDir))), nil
			}

			lspManager.Start(ctx, filepath.Dir(oldPath))
			client := findLSPClient(lspManager, oldPath)
			if client == nil {
				return fantasy.NewTextErrorResponse(
					"no LSP client handles this file, so there is nothing this tool can check for cascading edits -- use the write or bash tools for a plain move",
				), nil
			}

			edit, err := client.WillRenameFiles(ctx, oldPath, newPath)
			if err != nil && !isMethodNotFoundError(err) {
				slog.Error("willRenameFiles request failed", "error", err, "old_path", oldPath, "new_path", newPath)
				return fantasy.NewTextErrorResponse(fmt.Sprintf("willRenameFiles failed: %s", err)), nil
			}

			affectedFiles := collectAffectedFiles(edit)

			sessionID := GetSessionFromContext(ctx)
			if sessionID != "" {
				desc := fmt.Sprintf("Move %s to %s", relOrAbs(oldPath, workingDir), relOrAbs(newPath, workingDir))
				if len(affectedFiles) > 0 {
					desc = fmt.Sprintf("%s, updating references in %d other file(s)", desc, len(affectedFiles))
				}
				granted, err := permissions.Request(ctx, permission.CreatePermissionRequest{
					SessionID:   sessionID,
					ToolCallID:  call.ID,
					ToolName:    LspRenameFileToolName,
					Action:      "rename",
					Description: desc,
				})
				if err != nil {
					return fantasy.ToolResponse{}, err
				}
				if !granted {
					return NewPermissionDeniedResponse(permissions), nil
				}
			}

			if files != nil && sessionID != "" {
				for _, path := range affectedFiles {
					content, err := os.ReadFile(path)
					if err != nil {
						slog.Warn("Failed to read file for version tracking", "path", path, "error", err)
						continue
					}
					if _, err := files.CreateVersion(ctx, sessionID, path, string(content), GetMessageFromContext(ctx)); err != nil {
						slog.Warn("Failed to create file version", "path", path, "error", err)
					}
				}
			}

			if edit != nil && !workspaceEditEmpty(edit) {
				if err := lsputil.ApplyWorkspaceEdit(*edit, client.GetOffsetEncoding()); err != nil {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to apply cascading edits: %s", err)), nil
				}
			}

			// The edit above updates *other* files' references; the move
			// itself is the client's job unless the server unusually
			// already performed it as part of the edit (checked here so
			// this never double-moves or errors on an already-done move).
			if _, err := os.Stat(newPath); err != nil {
				if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
					return fantasy.ToolResponse{}, err
				}
				if err := os.Rename(oldPath, newPath); err != nil {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to move the file: %s", err)), nil
				}
			}

			if err := client.DidRenameFiles(ctx, oldPath, newPath); err != nil {
				slog.Warn("didRenameFiles notification failed", "error", err)
			}

			if filetracker != nil && sessionID != "" {
				for _, path := range affectedFiles {
					filetracker.RecordRead(ctx, sessionID, path)
				}
			}

			notifyLSPs(ctx, lspManager, "")

			var b strings.Builder
			fmt.Fprintf(&b, "Moved %s to %s.\n", relOrAbs(oldPath, workingDir), relOrAbs(newPath, workingDir))
			if len(affectedFiles) == 0 {
				b.WriteString("No other files needed updating.\n")
			} else {
				fmt.Fprintf(&b, "Updated %d other file(s):\n", len(affectedFiles))
				for _, f := range affectedFiles {
					fmt.Fprintf(&b, "  %s\n", relOrAbs(f, workingDir))
				}
			}

			return fantasy.NewTextResponse(b.String()), nil
		},
	)
}

func resolvePath(workingDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workingDir, path)
}

func isMethodNotFoundError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "method not found")
}

func workspaceEditEmpty(edit *protocol.WorkspaceEdit) bool {
	return len(edit.Changes) == 0 && len(edit.DocumentChanges) == 0
}
