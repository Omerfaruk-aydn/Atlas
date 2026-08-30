package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/omerfarukaydin/atlas/internal/tools"
)

const (
	fetchTimeout   = 20 * time.Second
	maxFetchBytes  = 256 * 1024
	maxOutputChars = 20000
)

type FetchTool struct{}

func (FetchTool) Name() string { return "fetch_url" }
func (FetchTool) Description() string {
	return "Bir URL'nin içeriğini getirir ve okunabilir metne dönüştürerek döndürür."
}
func (FetchTool) RequiresApproval() bool { return false }

func (FetchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {"type": "string", "description": "Getirilecek tam URL (http:// veya https://)"}
		},
		"required": ["url"]
	}`)
}

// Go's regexp engine (RE2) has no backreferences, so script/style blocks
// need one pattern each rather than a single "</\1>" closing-tag match.
var (
	scriptTag  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script\s*>`)
	styleTag   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style\s*>`)
	htmlTag    = regexp.MustCompile(`(?s)<[^>]+>`)
	multiBlank = regexp.MustCompile(`\n{3,}`)
)

func (FetchTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return tools.Result{Content: "geçersiz girdi: " + err.Error(), IsError: true}, nil
	}
	if !strings.HasPrefix(args.URL, "http://") && !strings.HasPrefix(args.URL, "https://") {
		return tools.Result{Content: "url http:// veya https:// ile başlamalı", IsError: true}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
	if err != nil {
		return tools.Result{Content: err.Error(), IsError: true}, nil
	}
	req.Header.Set("User-Agent", "Atlas-Agent/0.1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return tools.Result{Content: err.Error(), IsError: true}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return tools.Result{Content: err.Error(), IsError: true}, nil
	}

	if resp.StatusCode >= 400 {
		return tools.Result{Content: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), IsError: true}, nil
	}

	text := string(body)
	if strings.Contains(resp.Header.Get("Content-Type"), "html") {
		text = htmlToText(text)
	}
	if len(text) > maxOutputChars {
		text = text[:maxOutputChars] + "\n\n[...kesildi]"
	}

	return tools.Result{Content: text}, nil
}

// htmlToText is a minimal, dependency-free HTML→text conversion: strip
// script/style blocks and tags, then collapse whitespace. Good enough for
// feeding page content to a model; not a full HTML parser.
func htmlToText(html string) string {
	html = scriptTag.ReplaceAllString(html, "")
	html = styleTag.ReplaceAllString(html, "")
	text := htmlTag.ReplaceAllString(html, "\n")
	text = multiBlank.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
