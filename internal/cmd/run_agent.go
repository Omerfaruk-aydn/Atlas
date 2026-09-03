package cmd

import (
	"fmt"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
)

// wrapPromptForAgent validates that name is a configured subagent (and, if
// it names a model role, that the role resolves) and rewrites prompt into
// an instruction that routes the work through the "agent" tool with that
// name -- see internal/agent/agent_tool.go's agent_name parameter.
//
// `atlas run --agent` does not bypass the coder agent to run the subagent
// directly: the one-shot run still starts as an ordinary coder-agent turn,
// which is asked to delegate via the tool, the same way a model choosing to
// call it unprompted would. Failing early here means a typo'd name or an
// unresolvable model role is reported before any request is sent, instead
// of surfacing as a tool-call failure buried in the transcript.
func wrapPromptForAgent(cfg *config.Config, name, prompt string) (string, error) {
	var paths []string
	if cfg.Options != nil {
		paths = cfg.Options.SubagentsPaths
	}
	discovered := subagents.Discover(paths)
	sub, ok := subagents.Find(discovered, name)
	if !ok {
		return "", fmt.Errorf("no subagent named %q found; see `atlas agent list`", name)
	}
	if sub.Model != "" {
		if _, ok := cfg.ResolveRole(sub.Model); !ok {
			return "", fmt.Errorf("subagent %q references unknown model role %q; see `atlas models roles`", name, sub.Model)
		}
	}

	return fmt.Sprintf(
		"Use the agent tool with agent_name=%q to perform the following task, then report back its result:\n\n%s",
		name, prompt,
	), nil
}
