// Package k8sx reads Kubernetes YAML manifests and flags the handful of
// pod-spec mistakes that pass validation, deploy cleanly, and only show
// up later as an OOM-kill, a runaway container, or a wide-open security
// context.
//
// It parses generically (into map[string]any via yaml.v3) rather than
// against the real Kubernetes API types -- pulling in client-go for a
// handful of field reads would be a heavy dependency for what is,
// underneath, reading five or six well-known paths out of a YAML tree.
package k8sx

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Finding is one issue spotted in a manifest.
type Finding struct {
	// Kind is one of "missing-resource-limits", "latest-tag",
	// "privileged-container", "missing-probes", "host-namespace".
	Kind string
	// ObjectKind and ObjectName identify the manifest object, e.g.
	// "Deployment" and "web".
	ObjectKind string
	ObjectName string
	// Container is the container this finding is about, empty for a
	// pod-spec-level finding like host-namespace.
	Container string
	Message   string
}

// Result is the outcome of scanning one file, which may contain several
// "---"-separated documents.
type Result struct {
	Findings       []Finding
	ObjectsScanned int
}

// workloadKinds are the object kinds whose pod spec is worth checking.
// Anything else (ConfigMap, Service, Ingress, ...) has no containers to
// look at.
var workloadKinds = map[string]bool{
	"Pod": true, "Deployment": true, "StatefulSet": true,
	"DaemonSet": true, "Job": true, "ReplicaSet": true,
}

// Parse reads path, which may hold multiple "---"-separated YAML
// documents, and checks every workload object it contains.
func Parse(path string) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}

	var result Result
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var doc map[string]any
		err := dec.Decode(&doc)
		if err != nil {
			break // io.EOF ends the loop; a malformed later document just stops here.
		}
		if doc == nil {
			continue
		}

		kind, _ := doc["kind"].(string)
		if !workloadKinds[kind] {
			continue
		}
		result.ObjectsScanned++
		result.Findings = append(result.Findings, checkObject(kind, doc)...)
	}
	return result, nil
}

func checkObject(kind string, doc map[string]any) []Finding {
	name := objectName(doc)
	podSpec := podSpecOf(kind, doc)
	if podSpec == nil {
		return nil
	}

	var findings []Finding
	if truthy(podSpec["hostNetwork"]) {
		findings = append(findings, Finding{
			Kind: "host-namespace", ObjectKind: kind, ObjectName: name,
			Message: "hostNetwork: true shares the node's network namespace with the container",
		})
	}
	if truthy(podSpec["hostPID"]) {
		findings = append(findings, Finding{
			Kind: "host-namespace", ObjectKind: kind, ObjectName: name,
			Message: "hostPID: true shares the node's process namespace with the container",
		})
	}

	for _, c := range containersOf(podSpec) {
		findings = append(findings, checkContainer(kind, name, c)...)
	}
	return findings
}

func checkContainer(objKind, objName string, c map[string]any) []Finding {
	name, _ := c["name"].(string)
	var findings []Finding

	if f := checkResourceLimits(objKind, objName, name, c); f != nil {
		findings = append(findings, *f)
	}
	if f := checkImageTag(objKind, objName, name, c); f != nil {
		findings = append(findings, *f)
	}
	if f := checkPrivileged(objKind, objName, name, c); f != nil {
		findings = append(findings, *f)
	}
	if objKind == "Deployment" || objKind == "StatefulSet" || objKind == "DaemonSet" {
		findings = append(findings, checkProbes(objKind, objName, name, c)...)
	}
	return findings
}

func checkResourceLimits(objKind, objName, container string, c map[string]any) *Finding {
	resources, _ := c["resources"].(map[string]any)
	limits, _ := resources["limits"].(map[string]any)
	if len(limits) > 0 {
		return nil
	}
	return &Finding{
		Kind: "missing-resource-limits", ObjectKind: objKind, ObjectName: objName, Container: container,
		Message: "no resources.limits set -- this container can consume unbounded CPU or memory on the node",
	}
}

func checkImageTag(objKind, objName, container string, c map[string]any) *Finding {
	image, _ := c["image"].(string)
	if image == "" || strings.Contains(image, "@") {
		return nil
	}
	lastSlash := strings.LastIndex(image, "/")
	tagPart := image
	if lastSlash >= 0 {
		tagPart = image[lastSlash+1:]
	}
	if !strings.Contains(tagPart, ":") {
		return &Finding{
			Kind: "latest-tag", ObjectKind: objKind, ObjectName: objName, Container: container,
			Message: fmt.Sprintf("image %s has no tag, which resolves to :latest", image),
		}
	}
	if strings.HasSuffix(tagPart, ":latest") {
		return &Finding{
			Kind: "latest-tag", ObjectKind: objKind, ObjectName: objName, Container: container,
			Message: fmt.Sprintf("image %s pins the mutable :latest tag", image),
		}
	}
	return nil
}

func checkPrivileged(objKind, objName, container string, c map[string]any) *Finding {
	sc, _ := c["securityContext"].(map[string]any)
	if !truthy(sc["privileged"]) {
		return nil
	}
	return &Finding{
		Kind: "privileged-container", ObjectKind: objKind, ObjectName: objName, Container: container,
		Message: "securityContext.privileged: true gives this container root-equivalent access to the host",
	}
}

func checkProbes(objKind, objName, container string, c map[string]any) []Finding {
	var findings []Finding
	if _, ok := c["livenessProbe"]; !ok {
		findings = append(findings, Finding{
			Kind: "missing-probes", ObjectKind: objKind, ObjectName: objName, Container: container,
			Message: "no livenessProbe -- Kubernetes cannot detect and restart a container that has hung",
		})
	}
	if _, ok := c["readinessProbe"]; !ok {
		findings = append(findings, Finding{
			Kind: "missing-probes", ObjectKind: objKind, ObjectName: objName, Container: container,
			Message: "no readinessProbe -- traffic can reach this container before it is ready to serve",
		})
	}
	return findings
}

func objectName(doc map[string]any) string {
	meta, _ := doc["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	if name == "" {
		return "(unnamed)"
	}
	return name
}

// podSpecOf navigates to the pod spec for the given kind: directly under
// spec for a bare Pod, or under spec.template.spec for anything that
// wraps a pod template.
func podSpecOf(kind string, doc map[string]any) map[string]any {
	spec, _ := doc["spec"].(map[string]any)
	if spec == nil {
		return nil
	}
	if kind == "Pod" {
		return spec
	}
	template, _ := spec["template"].(map[string]any)
	podSpec, _ := template["spec"].(map[string]any)
	return podSpec
}

func containersOf(podSpec map[string]any) []map[string]any {
	raw, _ := podSpec["containers"].([]any)
	var out []map[string]any
	for _, item := range raw {
		if c, ok := item.(map[string]any); ok {
			out = append(out, c)
		}
	}
	return out
}

func truthy(v any) bool {
	b, ok := v.(bool)
	return ok && b
}
