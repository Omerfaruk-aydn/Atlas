package shell

// JobEventType is the kind of lifecycle transition a JobEvent reports.
type JobEventType string

const (
	// JobEventStarted is published when a background shell starts.
	JobEventStarted JobEventType = "started"
	// JobEventCompleted is published exactly once when a background shell
	// stops running, whether it exited naturally or was killed.
	JobEventCompleted JobEventType = "completed"
)

// JobEvent is published by BackgroundShellManager on job lifecycle
// transitions. Consumers should treat it purely as a refresh signal and
// re-fetch via ListInfo rather than trusting the embedded snapshot for
// anything beyond deciding whether to redraw — mirrors how
// workspace.LSPEvent is used only to trigger a refresh.
type JobEvent struct {
	Type JobEventType
	Info BackgroundShellInfo
}
