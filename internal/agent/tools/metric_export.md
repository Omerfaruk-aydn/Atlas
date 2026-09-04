List every Prometheus metric constructed in a Go codebase: name, type, help text, and labels.

WHEN TO USE THIS TOOL:
- Getting an inventory of what a service already exposes before adding a new metric, to avoid a near-duplicate under a slightly different name
- Reviewing a PR that touches instrumentation, to see the full set of metrics at a glance instead of finding each `prometheus.New...` call by hand
- Writing a dashboard or alert and needing to confirm a metric's exact name and label set

WHAT IT FINDS:
Every call to `prometheus.NewCounter`, `NewCounterVec`, `NewGauge`, `NewGaugeVec`, `NewHistogram`, `NewHistogramVec`, `NewSummary`, or `NewSummaryVec` -- matched by method name, so `promauto.NewCounter(...)` and a chained `promauto.With(reg).NewCounter(...)` are both recognised the same as a plain `prometheus.NewCounter(...)`. A Vec variant's second argument (its label names) is read when it's a literal `[]string{...}`.

The reported name joins Namespace, Subsystem, and Name the way Prometheus itself does, e.g. `app_http_requests_total`.

PARAMETERS:
- path: directory or single file to scan. Defaults to the working directory.
- include_tests: also scan test files. Off by default.

WHAT THIS DOES NOT DO:
This reads the constructor call's arguments as literal syntax -- when Opts is built in a variable, a helper function, or assembled in a loop rather than written inline, the metric is still counted but reported with name `(unknown)` and no help text or labels, since following an arbitrary expression back to its literal values isn't something syntax alone can promise. It also does not check whether a metric is ever actually registered or observed, and it does not recognise other metrics libraries (OpenTelemetry, StatsD, ...).
