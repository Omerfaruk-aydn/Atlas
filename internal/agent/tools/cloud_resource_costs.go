package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/costestimate"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
)

const CloudResourceCostsToolName = "cloud_resource_costs"

//go:embed cloud_resource_costs.md
var cloudResourceCostsDescription string

type CloudResourceCostsParams struct {
	Path string `json:"path,omitempty" description:"Directory or single file to scan. Defaults to the working directory."`
}

type CloudResourceCostsResponseMetadata struct {
	TotalMonthlyUSD float64 `json:"total_monthly_usd"`
	Instances       int     `json:"instances"`
	K8sWorkloads    int     `json:"k8s_workloads"`
	FilesScanned    int     `json:"files_scanned"`
}

func NewCloudResourceCostsTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		CloudResourceCostsToolName,
		cloudResourceCostsDescription,
		func(ctx context.Context, params CloudResourceCostsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			result, err := costestimate.Estimate(root)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if result.FilesScanned == 0 {
				return fantasy.NewTextResponse("No .tf or .yaml/.yml files found to scan. Check the path."), nil
			}
			if len(result.Instances) == 0 && len(result.K8sWorkloads) == 0 {
				return fantasy.NewTextResponse(fmt.Sprintf(
					"No costable resources found across %d file(s) -- no aws_instance resources, and no Deployment/StatefulSet with resource requests set.",
					result.FilesScanned)), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatCostEstimate(result, workingDir)),
				CloudResourceCostsResponseMetadata{
					TotalMonthlyUSD: result.TotalMonthlyUSD,
					Instances:       len(result.Instances),
					K8sWorkloads:    len(result.K8sWorkloads),
					FilesScanned:    result.FilesScanned,
				},
			), nil
		},
	)
}

func formatCostEstimate(r costestimate.Result, workingDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Estimated ~$%.2f/month across %d file(s). Illustrative only -- see the tool description for why.\n", r.TotalMonthlyUSD, r.FilesScanned)

	if len(r.Instances) > 0 {
		b.WriteString("\nTerraform (aws_instance):\n")
		for _, inst := range r.Instances {
			rel := relOrAbs(inst.File, workingDir)
			if inst.Known {
				fmt.Fprintf(&b, "  %s (%s) x%d @ %s: ~$%.2f/month\n", inst.Name, rel, inst.Count, inst.InstanceType, inst.MonthlyUSD)
			} else {
				fmt.Fprintf(&b, "  %s (%s) x%d @ %s: unknown instance type, not priced\n", inst.Name, rel, inst.Count, inst.InstanceType)
			}
		}
	}
	if len(r.UnknownInstanceTypes) > 0 {
		fmt.Fprintf(&b, "\nUnknown instance types (not in the price table, excluded from the total): %s\n", strings.Join(r.UnknownInstanceTypes, ", "))
	}

	if len(r.K8sWorkloads) > 0 {
		b.WriteString("\nKubernetes (Deployment/StatefulSet requests):\n")
		for _, w := range r.K8sWorkloads {
			rel := relOrAbs(w.File, workingDir)
			fmt.Fprintf(&b, "  %s/%s (%s) x%d replicas, %.2f vCPU + %.2f GiB total: ~$%.2f/month\n",
				w.Kind, w.Name, rel, w.Replicas, w.CPUCores, w.MemoryGiB, w.MonthlyUSD)
		}
	}
	return b.String()
}
