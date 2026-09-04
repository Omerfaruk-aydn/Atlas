package tools

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/k8sx"
)

const K8sManifestLintToolName = "k8s_manifest_lint"

//go:embed k8s_manifest_lint.md
var k8sManifestLintDescription string

type K8sManifestLintParams struct {
	Path string `json:"path" description:"Path to the Kubernetes manifest file. Required."`
}

type K8sManifestLintResponseMetadata struct {
	ObjectsScanned int            `json:"objects_scanned"`
	Total          int            `json:"total"`
	ByKind         map[string]int `json:"by_kind"`
}

func NewK8sManifestLintTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		K8sManifestLintToolName,
		k8sManifestLintDescription,
		func(ctx context.Context, params K8sManifestLintParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Path) == "" {
				return fantasy.NewTextErrorResponse("path is required"), nil
			}
			path := params.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(workingDir, path)
			}

			result, err := k8sx.Parse(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("no manifest found at %s", relOrAbs(path, workingDir))), nil
				}
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if result.ObjectsScanned == 0 {
				return fantasy.NewTextResponse("No Pod, Deployment, StatefulSet, DaemonSet, Job, or ReplicaSet object found -- nothing to check."), nil
			}

			byKind := map[string]int{}
			for _, f := range result.Findings {
				byKind[f.Kind]++
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatK8sManifestLint(result)),
				K8sManifestLintResponseMetadata{
					ObjectsScanned: result.ObjectsScanned,
					Total:          len(result.Findings),
					ByKind:         byKind,
				},
			), nil
		},
	)
}

func formatK8sManifestLint(r k8sx.Result) string {
	var b strings.Builder

	if len(r.Findings) == 0 {
		fmt.Fprintf(&b, "No issues found across %d workload object(s).\n", r.ObjectsScanned)
		return b.String()
	}

	fmt.Fprintf(&b, "%d issue(s) across %d workload object(s).\n", len(r.Findings), r.ObjectsScanned)
	currentObject := ""
	for _, f := range r.Findings {
		object := fmt.Sprintf("%s/%s", f.ObjectKind, f.ObjectName)
		if object != currentObject {
			currentObject = object
			fmt.Fprintf(&b, "\n%s\n", object)
		}
		if f.Container != "" {
			fmt.Fprintf(&b, "  [%s] container %s: %s\n", f.Kind, f.Container, f.Message)
		} else {
			fmt.Fprintf(&b, "  [%s] %s\n", f.Kind, f.Message)
		}
	}
	return b.String()
}
