package agent

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/agent/tools"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/csync"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
)

//go:embed templates/orchestrate_tool.md
var orchestrateToolDescription string

const OrchestrateToolName = "orchestrate"

type OrchestrateParams struct {
	Prompt string `json:"prompt" description:"The task to hand to every named agent, verbatim. Each one receives the exact same prompt and runs independently."`
	// AgentNames names at least two configured subagents (see internal/subagents
	// and `atlas agent list`) to run this same prompt with, in parallel.
	AgentNames []string `json:"agent_names" description:"At least two distinct subagent names to run this prompt with, in parallel."`
}

type OrchestrateResponseMetadata struct {
	Agents []string `json:"agents"`
}

type orchestrateResult struct {
	name    string
	content string
	err     error
}

// orchestrateTool runs the same prompt on several named subagents in
// parallel and returns every answer side by side, so the calling model
// can cross-check them in one round trip instead of orchestrating
// several `agent` calls itself and comparing them by hand. It shares
// resolveSubagent/buildSubagentSessionAgent/runSubAgent with agentTool
// -- the only difference is fanning one prompt out to several agents
// instead of routing it to one.
func (c *coordinator) orchestrateTool(ctx context.Context) (fantasy.AgentTool, error) {
	agentCfg, ok := c.cfg.Config().Agents[config.AgentTask]
	if !ok {
		return nil, errors.New("task agent not configured")
	}

	var opts *config.Options
	if opts = c.cfg.Config().Options; opts == nil {
		opts = &config.Options{}
	}
	discovered := subagents.Discover(opts.SubagentsPaths)
	subagentInstances := csync.NewMap[string, SessionAgent]()

	// A limiter scoped to this tool, the same way agentTool's is: it
	// bounds how many of the agents named in one orchestrate call run at
	// once, using the same MaxConcurrentSubAgents setting.
	limiter := newConcurrencyLimiter(c.cfg.Config().Options.MaxConcurrentSubAgents)

	return fantasy.NewParallelAgentTool(
		OrchestrateToolName,
		orchestrateToolDescription,
		func(ctx context.Context, params OrchestrateParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Prompt) == "" {
				return fantasy.NewTextErrorResponse("prompt is required"), nil
			}

			names := dedupeAgentNames(params.AgentNames)
			if len(names) < 2 {
				return fantasy.NewTextErrorResponse(
					"orchestrate needs at least two distinct agent_names -- for a single agent, use `agent` instead"), nil
			}

			sessionID := tools.GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, errors.New("session id missing from context")
			}
			agentMessageID := tools.GetMessageFromContext(ctx)
			if agentMessageID == "" {
				return fantasy.ToolResponse{}, errors.New("agent message id missing from context")
			}

			results := make([]orchestrateResult, len(names))
			var wg sync.WaitGroup
			for i, name := range names {
				wg.Add(1)
				go func(i int, name string) {
					defer wg.Done()
					results[i] = c.runOrchestratedAgent(ctx, orchestratedAgentParams{
						agentCfg:          agentCfg,
						discovered:        discovered,
						subagentInstances: subagentInstances,
						limiter:           limiter,
						name:              name,
						sessionID:         sessionID,
						agentMessageID:    agentMessageID,
						toolCallID:        fmt.Sprintf("%s-%s", call.ID, name),
						prompt:            params.Prompt,
					})
				}(i, name)
			}
			wg.Wait()

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatOrchestrateResults(results)),
				OrchestrateResponseMetadata{Agents: names},
			), nil
		},
	), nil
}

type orchestratedAgentParams struct {
	agentCfg          config.Agent
	discovered        []*subagents.Subagent
	subagentInstances *csync.Map[string, SessionAgent]
	limiter           *concurrencyLimiter
	name              string
	sessionID         string
	agentMessageID    string
	toolCallID        string
	prompt            string
}

func (c *coordinator) runOrchestratedAgent(ctx context.Context, p orchestratedAgentParams) orchestrateResult {
	runAgent, err := c.resolveSubagent(ctx, p.agentCfg, p.discovered, p.subagentInstances, p.name)
	if err != nil {
		return orchestrateResult{name: p.name, err: err}
	}

	if err := p.limiter.acquire(ctx); err != nil {
		return orchestrateResult{name: p.name, err: err}
	}
	defer p.limiter.release()

	resp, err := c.runSubAgent(ctx, subAgentParams{
		Agent:          runAgent,
		SessionID:      p.sessionID,
		AgentMessageID: p.agentMessageID,
		ToolCallID:     p.toolCallID,
		Prompt:         p.prompt,
		SessionTitle:   p.name + " agent session",
	})
	if err != nil {
		return orchestrateResult{name: p.name, err: err}
	}
	return orchestrateResult{name: p.name, content: resp.Content}
}

// dedupeAgentNames trims and deduplicates names, preserving first-seen
// order, so a caller listing the same agent twice by mistake gets one
// real run rather than two identical ones counted as corroboration.
func dedupeAgentNames(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func formatOrchestrateResults(results []orchestrateResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d agent(s) ran independently on the same prompt.\n", len(results))
	for _, r := range results {
		fmt.Fprintf(&b, "\n=== %s ===\n", r.name)
		if r.err != nil {
			fmt.Fprintf(&b, "failed: %s\n", r.err)
			continue
		}
		b.WriteString(r.content)
		b.WriteString("\n")
	}
	return b.String()
}
