Produce an order-of-magnitude monthly cost estimate from infrastructure-as-code: EC2 instance types in Terraform, and CPU/memory requests in Kubernetes workloads.

WHEN TO USE THIS TOOL:
- Before applying new Terraform or Kubernetes manifests, to get a rough sense of what they'll add to the bill
- Reviewing an infrastructure PR that changes instance types, resource requests, or replica counts, to see the estimated cost delta

WHAT IT COVERS:
- Terraform: every `aws_instance` resource's `instance_type`, multiplied by a literal `count` when present (defaults to 1). Only instance types in a small built-in price table are costed; anything else is listed separately as unknown rather than silently treated as free.
- Kubernetes: every Deployment and StatefulSet's container `resources.requests` (cpu and memory), multiplied by `spec.replicas` (defaults to 1), converted through a generic blended rate per vCPU-hour and per-GiB-hour. A workload with no requests set has nothing to estimate from and is skipped, not assumed to cost zero.

PARAMETERS:
- path: directory or single file to scan. Defaults to the working directory. Both .tf and .yaml/.yml files under it are read.

HOW ACCURATE THIS IS, AND WHY:
This is illustrative, not a quote. There is no live pricing lookup -- no network access, and a static built-in table is transparent about what it does and doesn't know, where a service call succeeding or failing would silently change what gets estimated. Real prices vary by region, reserved or spot pricing, and change over time; the Kubernetes rate is a generic blended figure with no tie to any specific cloud or node type, since the actual node pricing a workload runs on isn't knowable from the manifest alone. Use this to compare the relative cost of two configurations or catch an order-of-magnitude surprise (an accidentally huge instance type, a replica count typo), not to forecast an actual bill.
