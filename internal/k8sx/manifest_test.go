package k8sx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func findK8sFinding(t *testing.T, findings []Finding, kind string) *Finding {
	t.Helper()
	for i := range findings {
		if findings[i].Kind == kind {
			return &findings[i]
		}
	}
	return nil
}

const deploymentFixture = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: app
          image: myapp
`

func TestParseFlagsMissingResourceLimits(t *testing.T) {
	path := writeManifest(t, deploymentFixture)

	got, err := Parse(path)
	require.NoError(t, err)
	f := findK8sFinding(t, got.Findings, "missing-resource-limits")
	require.NotNil(t, f)
	require.Equal(t, "web", f.ObjectName)
	require.Equal(t, "app", f.Container)
}

func TestParseAcceptsResourceLimits(t *testing.T) {
	path := writeManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: app
          image: myapp:1.0
          resources:
            limits:
              cpu: "500m"
              memory: "256Mi"
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
          readinessProbe:
            httpGet:
              path: /ready
              port: 8080
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Empty(t, got.Findings)
}

func TestParseFlagsLatestTag(t *testing.T) {
	path := writeManifest(t, deploymentFixture)

	got, err := Parse(path)
	require.NoError(t, err)
	require.NotNil(t, findK8sFinding(t, got.Findings, "latest-tag"))
}

func TestParseAcceptsADigestPin(t *testing.T) {
	path := writeManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: app
          image: myapp@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findK8sFinding(t, got.Findings, "latest-tag"))
}

func TestParseFlagsPrivilegedContainer(t *testing.T) {
	path := writeManifest(t, `
apiVersion: v1
kind: Pod
metadata:
  name: debug
spec:
  containers:
    - name: app
      image: myapp:1.0
      securityContext:
        privileged: true
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.NotNil(t, findK8sFinding(t, got.Findings, "privileged-container"))
}

func TestParseFlagsMissingProbesOnDeployment(t *testing.T) {
	path := writeManifest(t, deploymentFixture)

	got, err := Parse(path)
	require.NoError(t, err)
	findings := got.Findings
	count := 0
	for _, f := range findings {
		if f.Kind == "missing-probes" {
			count++
		}
	}
	require.Equal(t, 2, count) // liveness and readiness
}

func TestParseDoesNotCheckProbesOnBarePod(t *testing.T) {
	path := writeManifest(t, `
apiVersion: v1
kind: Pod
metadata:
  name: debug
spec:
  containers:
    - name: app
      image: myapp:1.0
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Nil(t, findK8sFinding(t, got.Findings, "missing-probes"))
}

func TestParseFlagsHostNetwork(t *testing.T) {
	path := writeManifest(t, `
apiVersion: v1
kind: Pod
metadata:
  name: debug
spec:
  hostNetwork: true
  containers:
    - name: app
      image: myapp:1.0
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.NotNil(t, findK8sFinding(t, got.Findings, "host-namespace"))
}

func TestParseSkipsNonWorkloadKinds(t *testing.T) {
	path := writeManifest(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: config
data:
  key: value
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Equal(t, 0, got.ObjectsScanned)
	require.Empty(t, got.Findings)
}

func TestParseHandlesMultiDocumentFile(t *testing.T) {
	path := writeManifest(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: config
data:
  key: value
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: app
          image: myapp:1.0
          resources:
            limits:
              cpu: "1"
`)

	got, err := Parse(path)
	require.NoError(t, err)
	require.Equal(t, 1, got.ObjectsScanned)
}

func TestParseReportsErrorForMissingFile(t *testing.T) {
	_, err := Parse(filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err)
}
