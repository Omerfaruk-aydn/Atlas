package tools

import (
	"fmt"
	"net/url"
	"strings"
)

// URLPolicy decides whether the fetch, download, and agentic-fetch tools
// may reach a given URL. It exists because the sub-agent behind
// agentic_fetch reaches URLs autonomously, with no permission prompt in
// its path (see NewWebFetchTool) -- an operator who wants a hard limit on
// where that agent can go has nothing else to reach for.
//
// The zero value permits everything, matching the tool's behavior before
// this existed.
type URLPolicy struct {
	// Allow, if non-empty, is the complete list of hosts a URL's host must
	// match (itself or a subdomain of) to be permitted. Empty means every
	// host is permitted, subject to Deny.
	Allow []string
	// Deny is checked first and always wins: a host matching an entry here
	// is rejected even if it also matches Allow.
	Deny []string
}

// Check reports whether rawURL may be fetched. It only inspects the URL's
// host against the configured lists -- it does not resolve DNS or inspect
// where a redirect might lead, so it is a name-based policy, not a network
// sandbox.
func (p URLPolicy) Check(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("URL has no host: %s", rawURL)
	}

	for _, denied := range p.Deny {
		if hostMatchesDomain(host, denied) {
			return fmt.Errorf("%s is on the blocked domain list (%s)", host, denied)
		}
	}

	if len(p.Allow) == 0 {
		return nil
	}
	for _, allowed := range p.Allow {
		if hostMatchesDomain(host, allowed) {
			return nil
		}
	}
	return fmt.Errorf("%s is not on the allowed domain list", host)
}

// hostMatchesDomain reports whether host is domain itself or a subdomain of
// it, case-insensitively. domain is matched as given: a caller that means
// to cover subdomains should configure the bare domain ("example.com"),
// which also matches "docs.example.com".
func hostMatchesDomain(host, domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}
