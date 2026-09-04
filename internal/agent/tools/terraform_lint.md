Scan Terraform (.tf) source for a small set of misconfigurations that are valid HCL, apply cleanly, and only turn into an incident later.

WHEN TO USE THIS TOOL:
- Reviewing a Terraform module or root configuration before `terraform apply`
- Auditing existing infrastructure code for the kind of gap that `terraform validate` does not catch, because these are all syntactically and semantically valid Terraform

WHAT IT FLAGS:
- open-ingress: an `ingress` block whose `cidr_blocks` includes `0.0.0.0/0` or `::/0` -- traffic allowed from the entire internet. The same range in an `egress` block is not flagged; allowing all outbound traffic is the common, usually intentional default.
- hardcoded-credential: a literal `access_key`, `secret_key`, `token`, or `password` set directly inside a `provider` block, or a `variable` whose name looks like a credential (password, secret, token, api_key) given a literal `default`. Either bakes a real credential into version-controlled source instead of pulling it from the environment or a secrets manager.
- public-acl: `acl = "public-read"` or `"public-read-write"` on any resource, most commonly an S3 bucket ACL -- makes the resource's contents publicly accessible.

PARAMETERS:
- path: directory or single .tf file to scan. Defaults to the working directory.

HOW THIS WORKS, AND ITS LIMITS:
This tracks block nesting by counting braces line by line rather than parsing full HCL -- accurate for `terraform fmt`-formatted source (the overwhelming majority of real Terraform), but a brace inside a string or several block headers sharing one line can throw the nesting off. It does not run `terraform plan` or `validate`, does not know whether a referenced module or variable actually resolves, and only checks the three patterns above -- a clean report means these specific mistakes are absent, not that the configuration is otherwise correct or secure.
