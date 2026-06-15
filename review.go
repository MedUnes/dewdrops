package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// HTTPDoer abstracts *http.Client so the review client is unit-testable without
// a network. Inject *http.Client in prod, a mock in tests.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// defaultReviewPrompt is the embedded system prompt used when --prompt-file is
// not supplied. --prompt-file overrides it.
const defaultReviewPrompt = `You are a senior code reviewer. You are given a merge request as a structure map,
a unified diff, and the full content of changed files.

Output concise Markdown in this order:
1. Blocking: correctness, security, data-loss, or breaking-change issues.
2. Suggestions:  design, readability, maintainability.
3. Nits:  style, naming, minor.

Rules:
- Cite file:line for every point.
- Be specific; no generic advice that would apply to any MR.
- If a section is empty, omit it. If the MR is clean, say so in one line.
- No preamble, no summary of what the MR does, reviewers can read the diff.`

const reviewUsage = `Usage: dewdrops review --since <ref> --model <name> --base-url <url> [options] <repo-path>

Generates the --since composite (map + diff + content), sends it to an
OpenAI-compatible /chat/completions endpoint, and writes the model's review to
stdout. This is the one online mode; every other dewdrops mode stays offline.

Required:
  --since <ref>        Git ref to diff against HEAD (same semantics as top-level --since).
  --model <name>       Model identifier sent in the request body. (or env DEWDROPS_MODEL)
  --base-url <url>     Endpoint base, e.g. https://host/v1/.... (or env DEWDROPS_BASE_URL)
                       The request goes to <base-url>/chat/completions.

Optional:
  --prompt-file <path> System prompt file. If omitted, uses the embedded default prompt.
  --max-tokens <int>   Max tokens for the response. Default 8192.
  --timeout <seconds>  HTTP timeout in seconds. Default 240.
  -h, --help           Show this help message.

Credentials (env only, never a flag):
  DEWDROPS_API_KEY     Sent as: Authorization: Bearer <key>. Required.
`

// reviewConfig holds the fully-resolved settings for one review invocation.
type reviewConfig struct {
	since        string
	repoPath     string
	model        string
	baseURL      string
	apiKey       string
	systemPrompt string
	maxTokens    int
	timeout      time.Duration
}

// runReview parses the `review` subcommand args, builds the --since composite,
// sends it to the LLM, and writes the review to stdout. It returns a process
// exit code (it never calls os.Exit), so it is fully testable.
//
// Exit codes: 0 success or no-changes; 2 usage/missing required config;
// 1 runtime failure (bad path, generation error, HTTP/parse/timeout error).
func runReview(args []string, getenv func(string) string, stdout, stderr io.Writer, newDoer func(time.Duration) HTTPDoer) int {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, reviewUsage) }

	since := fs.String("since", "", "Git ref to diff against HEAD")
	model := fs.String("model", "", "Model identifier (or env DEWDROPS_MODEL)")
	baseURL := fs.String("base-url", "", "OpenAI-compatible endpoint base URL (or env DEWDROPS_BASE_URL)")
	promptFile := fs.String("prompt-file", "", "System prompt file (overrides embedded default)")
	maxTokens := fs.Int("max-tokens", 8192, "Max tokens for the response")
	timeoutSec := fs.Int("timeout", 240, "HTTP timeout in seconds")

	if err := fs.Parse(args); err != nil {
		// -h/--help reports flag.ErrHelp and is not a failure.
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	modelVal := *model
	if modelVal == "" {
		modelVal = getenv("DEWDROPS_MODEL")
	}
	baseURLVal := *baseURL
	if baseURLVal == "" {
		baseURLVal = getenv("DEWDROPS_BASE_URL")
	}
	apiKey := getenv("DEWDROPS_API_KEY")
	repoPath := fs.Arg(0)

	var missing []string
	if *since == "" {
		missing = append(missing, "--since")
	}
	if modelVal == "" {
		missing = append(missing, "--model (or DEWDROPS_MODEL)")
	}
	if baseURLVal == "" {
		missing = append(missing, "--base-url (or DEWDROPS_BASE_URL)")
	}
	if repoPath == "" {
		missing = append(missing, "<repo-path>")
	}
	if apiKey == "" {
		missing = append(missing, "DEWDROPS_API_KEY (env)")
	}
	if len(missing) > 0 {
		fmt.Fprintf(stderr, "dewdrops review: missing required config: %s\n\n", strings.Join(missing, ", "))
		fmt.Fprint(stderr, reviewUsage)
		return 2
	}

	info, err := os.Stat(repoPath)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "dewdrops review: '%s' is not a valid directory\n", repoPath)
		return 1
	}

	systemPrompt := defaultReviewPrompt
	if *promptFile != "" {
		data, err := os.ReadFile(*promptFile)
		if err != nil {
			fmt.Fprintf(stderr, "dewdrops review: cannot read prompt file '%s': %v\n", *promptFile, err)
			return 1
		}
		systemPrompt = string(data)
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	stats, err := writeSinceOutput(w, repoPath, *since)
	if err != nil {
		fmt.Fprintf(stderr, "dewdrops review: %v\n", err)
		return 1
	}
	w.Flush()

	if stats.filesChanged == 0 {
		return 0
	}

	cfg := reviewConfig{
		since:        *since,
		repoPath:     repoPath,
		model:        modelVal,
		baseURL:      baseURLVal,
		apiKey:       apiKey,
		systemPrompt: systemPrompt,
		maxTokens:    *maxTokens,
		timeout:      time.Duration(*timeoutSec) * time.Second,
	}

	review, err := requestReview(newDoer(cfg.timeout), cfg, buf.String())
	if err != nil {
		fmt.Fprintf(stderr, "dewdrops review: %v\n", err)
		return 1
	}

	// Write straight to stdout (unbuffered) so the os.Exit(runReview(...))
	// caller can't drop a buffered tail.
	fmt.Fprint(stdout, review)
	return 0
}

// chatMessage / chatRequest model the OpenAI-compatible request body.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Messages  []chatMessage `json:"messages"`
}

// chatResponse models the subset of the response we read. content may be empty
// on reasoning models, which put output in reasoning_content instead.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// requestReview POSTs the composite and returns the extracted review text.
func requestReview(doer HTTPDoer, cfg reviewConfig, composite string) (string, error) {
	reqBody := chatRequest{
		Model:     cfg.model,
		MaxTokens: cfg.maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: cfg.systemPrompt},
			{Role: "user", Content: composite},
		},
	}
	// EXTENSION POINT (v2): the Anthropic /v1/messages API uses a different
	// shape, top-level "system" plus "messages" without a system role, and a
	// different path. Branch here (body + buildChatURL) on an endpoint kind.
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}

	url := buildChatURL(cfg.baseURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := doer.Do(req)
	if err != nil {
		// Keep timeout distinct from HTTP errors so callers can tell them apart.
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return "", fmt.Errorf("request timed out after %s: %w", cfg.timeout, err)
		}
		return "", fmt.Errorf("connection error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	return parseReviewResponse(resp.StatusCode, body)
}

// buildChatURL appends the chat-completions path, trimming one trailing slash so
// both "https://h/v1" and "https://h/v1/" resolve to "https://h/v1/chat/completions".
func buildChatURL(base string) string {
	return strings.TrimSuffix(base, "/") + "/chat/completions"
}

// parseReviewResponse extracts the review text, handling the edge cases that
// were discovered the hard way in production (see review_spec.md).
func parseReviewResponse(status int, body []byte) (string, error) {
	// Never hide the body on non-200: a max_tokens-too-large 400 must not look
	// like a timeout.
	if status != http.StatusOK {
		return "", fmt.Errorf("LLM request failed (HTTP %d): %s", status, snippet(body, 600))
	}

	var r chatResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("unparseable response (HTTP %d): %v: %s", status, err, snippet(body, 600))
	}

	if len(r.Choices) == 0 {
		// A 200 can still carry an error envelope and no choices.
		if r.Error != nil && r.Error.Message != "" {
			return "", fmt.Errorf("API error: %s", r.Error.Message)
		}
		return "", fmt.Errorf("response had no choices: %s", snippet(body, 600))
	}

	choice := r.Choices[0]
	if strings.TrimSpace(choice.Message.Content) != "" {
		return choice.Message.Content, nil
	}
	// Reasoning models (e.g. Qwen3) may leave content empty and put output in
	// reasoning_content instead.
	if strings.TrimSpace(choice.Message.ReasoningContent) != "" {
		return choice.Message.ReasoningContent, nil
	}
	// Empty content with no reasoning: surface finish_reason so the caller knows
	// to raise --max-tokens when it's "length".
	return "", fmt.Errorf("empty review (finish_reason: %s)", choice.FinishReason)
}

// snippet returns up to n bytes of b as a string, for error diagnostics.
func snippet(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}
