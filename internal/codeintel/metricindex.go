package codeintel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
)

// PromMetric is one Prometheus metric registration found in source.
type PromMetric struct {
	// Name is Namespace_Subsystem_Name joined per Prometheus convention,
	// or "(unknown)" when the opts weren't a literal this tool could
	// read.
	Name string
	// Type is "counter", "gauge", "histogram", or "summary".
	Type   string
	Help   string
	Labels []string
	File   string
	Line   int
}

// MetricIndexResult is the outcome of a scan.
type MetricIndexResult struct {
	Metrics      []PromMetric
	FilesScanned int
}

// metricConstructors maps a prometheus/promauto constructor's method
// name to the metric type it creates. The "Vec" variants take a second
// argument naming the metric's labels.
var metricConstructors = map[string]string{
	"NewCounter": "counter", "NewCounterVec": "counter",
	"NewGauge": "gauge", "NewGaugeVec": "gauge",
	"NewHistogram": "histogram", "NewHistogramVec": "histogram",
	"NewSummary": "summary", "NewSummaryVec": "summary",
}

// IndexMetrics walks root for Go source and lists every Prometheus
// metric constructed with prometheus.NewXxx or promauto.NewXxx (with or
// without a preceding .With(...) registerer call).
//
// A metric whose Opts argument isn't a literal at the call site --
// built in a variable, a helper function, or a loop -- is still counted,
// but with its name reported as "(unknown)" and its Help and Labels
// empty, since reading through an arbitrary expression back to its
// literal values is not something syntax alone can promise.
func IndexMetrics(root string, includeTests bool) (MetricIndexResult, error) {
	files, err := collectGoFiles(root, includeTests)
	if err != nil {
		return MetricIndexResult{}, err
	}

	result := MetricIndexResult{}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		result.FilesScanned++

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			metricType, ok := metricConstructors[sel.Sel.Name]
			if !ok || len(call.Args) == 0 {
				return true
			}

			m := PromMetric{
				Type: metricType,
				File: path,
				Line: fset.Position(call.Pos()).Line,
			}
			fields := compositeLitFields(call.Args[0])
			m.Name = combineName(fields["namespace"], fields["subsystem"], fields["name"])
			m.Help = fields["help"]
			if strings.HasSuffix(sel.Sel.Name, "Vec") && len(call.Args) > 1 {
				m.Labels = stringSliceLit(call.Args[1])
			}
			result.Metrics = append(result.Metrics, m)
			return true
		})
	}

	sort.SliceStable(result.Metrics, func(i, j int) bool {
		if result.Metrics[i].File != result.Metrics[j].File {
			return result.Metrics[i].File < result.Metrics[j].File
		}
		return result.Metrics[i].Line < result.Metrics[j].Line
	})
	return result, nil
}

func combineName(namespace, subsystem, name string) string {
	var parts []string
	for _, p := range []string{namespace, subsystem, name} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "(unknown)"
	}
	return strings.Join(parts, "_")
}

// compositeLitFields reads string-valued fields out of a struct literal
// like prometheus.CounterOpts{Name: "x", Help: "y"}. Keys are matched
// case-insensitively and returned lower-cased. A non-literal argument
// (a variable, a function call) yields an empty map rather than an
// error -- there is nothing more to read from syntax alone.
func compositeLitFields(expr ast.Expr) map[string]string {
	fields := map[string]string{}
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return fields
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		if v, ok := stringLitValue(kv.Value); ok {
			fields[strings.ToLower(key.Name)] = v
		}
	}
	return fields
}

func stringLitValue(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return v, true
}

func stringSliceLit(expr ast.Expr) []string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var out []string
	for _, elt := range lit.Elts {
		if v, ok := stringLitValue(elt); ok {
			out = append(out, v)
		}
	}
	return out
}
