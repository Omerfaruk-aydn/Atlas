// Package costestimate produces an order-of-magnitude monthly cost
// estimate from infrastructure-as-code source: EC2 instance types in
// Terraform, and CPU/memory requests in Kubernetes workloads.
//
// This is deliberately not connected to any pricing API -- no network
// access, and a live lookup would make the estimate's accuracy depend on
// a service call succeeding rather than on the numbers this package
// ships with. Instead it carries a small, static table of approximate
// on-demand rates. Both the table and the blended Kubernetes rate are
// illustrative: real prices vary by region, commitment discounts, and
// time, so treat every number here as "roughly this order of magnitude
// today," not a quote.
package costestimate

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// hoursPerMonth approximates a month as 365*24/12, the same convention
// cloud billing calculators use.
const hoursPerMonth = 730.0

// Generic blended Kubernetes compute rates: illustrative, not tied to
// any specific cloud or instance family. A real cluster's actual cost
// per vCPU and per GB depends on the node types it runs on.
const (
	k8sHourlyPerVCPU   = 0.04
	k8sHourlyPerGiBRAM = 0.005
)

// ec2HourlyUSD is an approximate, illustrative on-demand hourly rate for
// common instance types (us-east-1-like pricing). Anything not listed is
// reported as unknown rather than guessed at.
var ec2HourlyUSD = map[string]float64{
	"t3.nano": 0.0052, "t3.micro": 0.0104, "t3.small": 0.0208,
	"t3.medium": 0.0416, "t3.large": 0.0832, "t3.xlarge": 0.1664, "t3.2xlarge": 0.3328,
	"m5.large": 0.096, "m5.xlarge": 0.192, "m5.2xlarge": 0.384, "m5.4xlarge": 0.768,
	"c5.large": 0.085, "c5.xlarge": 0.17, "c5.2xlarge": 0.34, "c5.4xlarge": 0.68,
	"r5.large": 0.126, "r5.xlarge": 0.252, "r5.2xlarge": 0.504,
}

// InstanceEstimate is one Terraform aws_instance resource.
type InstanceEstimate struct {
	Name         string
	InstanceType string
	// Count is how many instances this resource creates. 1 unless a
	// literal "count = N" was found.
	Count      int
	HourlyUSD  float64
	MonthlyUSD float64
	// Known reports whether InstanceType was found in the price table.
	// When false, HourlyUSD and MonthlyUSD are zero and this instance is
	// excluded from the total rather than silently treated as free.
	Known bool
	File  string
}

// K8sWorkloadEstimate is one Kubernetes Deployment or StatefulSet.
type K8sWorkloadEstimate struct {
	Kind       string
	Name       string
	Replicas   int
	CPUCores   float64 // Total requested across all replicas.
	MemoryGiB  float64 // Total requested across all replicas.
	MonthlyUSD float64
	File       string
}

// Result is a combined estimate across every file scanned.
type Result struct {
	Instances            []InstanceEstimate
	K8sWorkloads         []K8sWorkloadEstimate
	TotalMonthlyUSD      float64
	UnknownInstanceTypes []string
	FilesScanned         int
}

// Estimate walks root for .tf and .yaml/.yml files and produces a
// combined cost estimate.
func Estimate(root string) (Result, error) {
	files, err := collectSourceFiles(root)
	if err != nil {
		return Result{}, err
	}

	result := Result{}
	unknownSeen := map[string]bool{}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		result.FilesScanned++

		switch {
		case strings.HasSuffix(path, ".tf"):
			for _, inst := range extractInstances(path, string(data)) {
				result.Instances = append(result.Instances, inst)
				if inst.Known {
					result.TotalMonthlyUSD += inst.MonthlyUSD
				} else if !unknownSeen[inst.InstanceType] {
					unknownSeen[inst.InstanceType] = true
					result.UnknownInstanceTypes = append(result.UnknownInstanceTypes, inst.InstanceType)
				}
			}
		case strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
			for _, w := range extractK8sWorkloads(path, data) {
				result.K8sWorkloads = append(result.K8sWorkloads, w)
				result.TotalMonthlyUSD += w.MonthlyUSD
			}
		}
	}

	sort.Strings(result.UnknownInstanceTypes)
	return result, nil
}

var (
	tfResourceHeader = regexp.MustCompile(`^resource\s+"aws_instance"\s+"([^"]+)"\s*\{`)
	tfInstanceType   = regexp.MustCompile(`^instance_type\s*=\s*"([^"]+)"`)
	tfCount          = regexp.MustCompile(`^count\s*=\s*(\d+)`)
)

// extractInstances reads aws_instance resources out of one .tf file's
// content using the same brace-depth tracking terraformx uses -- exact
// for terraform fmt-formatted source, approximate otherwise.
func extractInstances(path, content string) []InstanceEstimate {
	var out []InstanceEstimate
	var current *InstanceEstimate
	depth, blockDepth := 0, -1

	for _, raw := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		if m := tfResourceHeader.FindStringSubmatch(trimmed); m != nil {
			if current != nil {
				out = append(out, finishInstance(*current))
			}
			current = &InstanceEstimate{Name: m[1], Count: 1, File: path}
			blockDepth = depth
		} else if current != nil {
			if m := tfInstanceType.FindStringSubmatch(trimmed); m != nil {
				current.InstanceType = m[1]
			} else if m := tfCount.FindStringSubmatch(trimmed); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil {
					current.Count = n
				}
			}
		}

		depth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
		if current != nil && depth <= blockDepth {
			out = append(out, finishInstance(*current))
			current = nil
		}
	}
	if current != nil {
		out = append(out, finishInstance(*current))
	}
	return out
}

func finishInstance(inst InstanceEstimate) InstanceEstimate {
	if inst.InstanceType == "" {
		return inst
	}
	rate, ok := ec2HourlyUSD[inst.InstanceType]
	if !ok {
		return inst
	}
	inst.Known = true
	inst.HourlyUSD = rate
	inst.MonthlyUSD = rate * hoursPerMonth * float64(inst.Count)
	return inst
}

var k8sWorkloadKinds = map[string]bool{"Deployment": true, "StatefulSet": true}

func extractK8sWorkloads(path string, data []byte) []K8sWorkloadEstimate {
	var out []K8sWorkloadEstimate
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var doc map[string]any
		if err := dec.Decode(&doc); err != nil {
			break
		}
		if doc == nil {
			continue
		}
		kind, _ := doc["kind"].(string)
		if !k8sWorkloadKinds[kind] {
			continue
		}

		meta, _ := doc["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		if name == "" {
			name = "(unnamed)"
		}

		spec, _ := doc["spec"].(map[string]any)
		replicas := 1
		if r, ok := spec["replicas"].(int); ok && r > 0 {
			replicas = r
		}

		template, _ := spec["template"].(map[string]any)
		podSpec, _ := template["spec"].(map[string]any)
		containers, _ := podSpec["containers"].([]any)

		var cpu, memGiB float64
		for _, c := range containers {
			container, ok := c.(map[string]any)
			if !ok {
				continue
			}
			resources, _ := container["resources"].(map[string]any)
			requests, _ := resources["requests"].(map[string]any)
			if cpuStr, ok := requests["cpu"].(string); ok {
				cpu += parseCPU(cpuStr)
			}
			if memStr, ok := requests["memory"].(string); ok {
				memGiB += parseMemoryGiB(memStr)
			}
		}
		if cpu == 0 && memGiB == 0 {
			continue // No requests set -- nothing to estimate from.
		}

		totalCPU := cpu * float64(replicas)
		totalMem := memGiB * float64(replicas)
		monthly := (totalCPU*k8sHourlyPerVCPU + totalMem*k8sHourlyPerGiBRAM) * hoursPerMonth

		out = append(out, K8sWorkloadEstimate{
			Kind: kind, Name: name, Replicas: replicas,
			CPUCores: totalCPU, MemoryGiB: totalMem, MonthlyUSD: monthly, File: path,
		})
	}
	return out
}

func parseCPU(s string) float64 {
	s = strings.TrimSpace(s)
	if after, ok := strings.CutSuffix(s, "m"); ok {
		n, err := strconv.ParseFloat(after, 64)
		if err != nil {
			return 0
		}
		return n / 1000
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return n
}

func parseMemoryGiB(s string) float64 {
	s = strings.TrimSpace(s)
	units := []struct {
		suffix string
		toGiB  float64
	}{
		{"Ki", 1.0 / (1024 * 1024)},
		{"Mi", 1.0 / 1024},
		{"Gi", 1},
		{"Ti", 1024},
		{"K", 1e3 / (1 << 30)},
		{"M", 1e6 / (1 << 30)},
		{"G", 1e9 / (1 << 30)},
		{"T", 1e12 / (1 << 30)},
	}
	for _, u := range units {
		if after, ok := strings.CutSuffix(s, u.suffix); ok {
			n, err := strconv.ParseFloat(after, 64)
			if err != nil {
				return 0
			}
			return n * u.toGiB
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return n / (1 << 30) // Bare number is bytes, per the Kubernetes resource-quantity spec.
}

func collectSourceFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{root}, nil
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".terraform" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(d.Name())
		if strings.HasSuffix(lower, ".tf") || strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
