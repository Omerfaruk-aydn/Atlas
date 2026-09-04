package proto

import "time"

// BackgroundJob is the wire representation of a shell.BackgroundShellInfo.
type BackgroundJob struct {
	ID          string    `json:"id"`
	Command     string    `json:"command"`
	Description string    `json:"description"`
	WorkingDir  string    `json:"working_dir"`
	StartedAt   time.Time `json:"started_at"`
	Done        bool      `json:"done"`
	Status      string    `json:"status"`
	ExitErr     string    `json:"exit_err,omitempty"`
}

// SubAgentRun is the wire representation of an in-flight sub-agent run.
type SubAgentRun struct {
	SessionID string    `json:"session_id"`
	Title     string    `json:"title"`
	StartedAt time.Time `json:"started_at"`
}

// AgentHubEntry is the wire representation of a workspace.AgentHubEntry --
// one sub-agent session spawned under a top-level session, finished or
// still running.
type AgentHubEntry struct {
	SessionID    string    `json:"session_id"`
	Title        string    `json:"title"`
	StartedAt    time.Time `json:"started_at"`
	Cost         float64   `json:"cost"`
	MessageCount int64     `json:"message_count"`
	Busy         bool      `json:"busy"`
}

// JobEvent is the wire representation of a shell.JobEvent — a refresh
// signal for the background jobs sidebar, forwarded over SSE. Consumers
// re-fetch via GetJobs rather than trusting this payload for anything
// beyond triggering that refresh.
type JobEvent struct {
	Type string `json:"type"`
}
