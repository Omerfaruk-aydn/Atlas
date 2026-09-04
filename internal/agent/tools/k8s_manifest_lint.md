Scan a Kubernetes YAML manifest for pod-spec mistakes that pass `kubectl apply` cleanly and only surface later as an OOM-kill, a runaway container, or a wide-open security context.

WHEN TO USE THIS TOOL:
- Reviewing a Deployment, StatefulSet, DaemonSet, Job, or Pod manifest before it merges or deploys
- Auditing an existing manifest tree for the kind of gap that `kubectl apply --dry-run` does not catch, because these are all valid Kubernetes -- just risky defaults

WHAT IT FLAGS, PER CONTAINER:
- missing-resource-limits: no `resources.limits` set, so the container can consume unbounded CPU or memory on its node.
- latest-tag: an image with no tag, or an explicit `:latest` -- both float to whatever the registry currently serves. A digest pin (`@sha256:...`) is accepted.
- privileged-container: `securityContext.privileged: true`, which gives the container root-equivalent access to the host.
- missing-probes: on a Deployment, StatefulSet, or DaemonSet, no `livenessProbe` and/or no `readinessProbe` -- Kubernetes can neither restart a hung container nor hold traffic back from one that isn't ready. Not checked on a bare Pod, where there's no controller to act on a probe result anyway.

AND, AT THE POD-SPEC LEVEL:
- host-namespace: `hostNetwork: true` or `hostPID: true`, either of which shares a normally-isolated namespace with the node.

PARAMETERS:
- path: path to the manifest file. A file with multiple "---"-separated documents is fully scanned; only workload kinds (Pod, Deployment, StatefulSet, DaemonSet, Job, ReplicaSet) are checked, everything else (ConfigMap, Service, Ingress, ...) is skipped since it has no containers.

WHAT THIS DOES NOT DO:
It does not validate the manifest against the Kubernetes API schema, does not know whether referenced ConfigMaps or Secrets exist, and does not check anything beyond the five signals above -- a clean report means these specific mistakes are absent, not that the manifest is otherwise correct.
