package tools

import (
	"slices"
	"sort"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/shell"
)

// argumentBlock is one of the built-in subcommand blocks, kept as data so a
// CommandPolicy can drop the ones whose command the user has allowed.
type argumentBlock struct {
	cmd   string
	args  []string
	flags []string
}

var argumentBlocks = []argumentBlock{
	// System package managers
	{cmd: "apk", args: []string{"add"}},
	{cmd: "apt", args: []string{"install"}},
	{cmd: "apt-get", args: []string{"install"}},
	{cmd: "dnf", args: []string{"install"}},
	{cmd: "pacman", flags: []string{"-S"}},
	{cmd: "pkg", args: []string{"install"}},
	{cmd: "yum", args: []string{"install"}},
	{cmd: "zypper", args: []string{"install"}},

	// Language-specific package managers
	{cmd: "brew", args: []string{"install"}},
	{cmd: "cargo", args: []string{"install"}},
	{cmd: "gem", args: []string{"install"}},
	{cmd: "go", args: []string{"install"}},
	{cmd: "npm", args: []string{"install"}, flags: []string{"--global"}},
	{cmd: "npm", args: []string{"install"}, flags: []string{"-g"}},
	{cmd: "pip", args: []string{"install"}, flags: []string{"--user"}},
	{cmd: "pip3", args: []string{"install"}, flags: []string{"--user"}},
	{cmd: "pnpm", args: []string{"add"}, flags: []string{"--global"}},
	{cmd: "pnpm", args: []string{"add"}, flags: []string{"-g"}},
	{cmd: "yarn", args: []string{"global", "add"}},

	// `go test -exec` can run arbitrary commands
	{cmd: "go", args: []string{"test"}, flags: []string{"-exec"}},
}

// CommandPolicy adjusts the bash tool's built-in block list. Allow removes a
// command from it -- including that command's subcommand blocks, since
// allowing a command the agent could not usefully run would be a trap. Block
// adds commands the built-in list does not cover.
//
// Allow wins over Block for the same name: an explicit allow is the more
// specific instruction, and silently blocking a command the user allowed
// would be the worse failure.
type CommandPolicy struct {
	Allow []string
	Block []string
}

// NewCommandPolicy reads the policy out of the config's options.
func NewCommandPolicy(cfg *config.Config) CommandPolicy {
	if cfg == nil || cfg.Options == nil {
		return CommandPolicy{}
	}
	return CommandPolicy{
		Allow: cfg.Options.AllowedCommands,
		Block: cfg.Options.BlockedCommands,
	}
}

func (p CommandPolicy) allows(cmd string) bool {
	for _, a := range p.Allow {
		if strings.EqualFold(strings.TrimSpace(a), cmd) {
			return true
		}
	}
	return false
}

// banned returns the effective banned-command list: the built-in one minus
// everything allowed, plus everything blocked. The result is sorted and
// deduplicated so the tool description reads the same way every run.
func (p CommandPolicy) banned() []string {
	seen := make(map[string]struct{}, len(bannedCommands)+len(p.Block))
	var out []string
	add := func(cmd string) {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" || p.allows(cmd) {
			return
		}
		if _, ok := seen[cmd]; ok {
			return
		}
		seen[cmd] = struct{}{}
		out = append(out, cmd)
	}
	for _, cmd := range bannedCommands {
		add(cmd)
	}
	for _, cmd := range p.Block {
		add(cmd)
	}
	sort.Strings(out)
	return out
}

func (p CommandPolicy) blockFuncs() []shell.BlockFunc {
	funcs := []shell.BlockFunc{shell.CommandsBlocker(p.banned())}
	for _, b := range argumentBlocks {
		if p.allows(b.cmd) {
			continue
		}
		funcs = append(funcs, shell.ArgumentsBlocker(b.cmd, slices.Clone(b.args), slices.Clone(b.flags)))
	}
	return funcs
}
