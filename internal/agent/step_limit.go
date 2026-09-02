package agent

import "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"

// maxStepsReached reports whether a turn has taken max or more steps and
// should be stopped, independent of whether it is making progress. It is a
// blunter guard than hasRepeatedToolCalls: that one catches a turn stuck
// repeating itself, this one catches a turn that is making steady but
// unbounded progress -- a long chain of distinct tool calls that never
// loops but also never stops, which nothing else here would flag.
//
// max <= 0 means unbounded; this always returns false in that case.
func maxStepsReached(steps []fantasy.StepResult, max int) bool {
	if max <= 0 {
		return false
	}
	return len(steps) >= max
}
