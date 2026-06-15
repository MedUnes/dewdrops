package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test doubles ---

// mockDoer is an in-memory HTTPDoer. It records whether it was called and
// captures the outbound request and body, then returns a canned response (or err).
type mockDoer struct {
	called  bool
	gotReq  *http.Request
	gotBody []byte
	status  int
	resp    string
	err     error
}

func (m *mockDoer) Do(req *http.Request) (*http.Response, error) {
	m.called = true
	m.gotReq = req
	if req.Body != nil {
		m.gotBody, _ = io.ReadAll(req.Body)
	}
	if m.err != nil {
		return nil, m.err
	}
	return &http.Response{
		StatusCode: m.status,
		Body:       io.NopCloser(bytes.NewReader([]byte(m.resp))),
		Header:     make(http.Header),
	}, nil
}

// timeoutErr satisfies net.Error with Timeout()==true, mimicking an http.Client
// deadline. Production wraps this in *url.Error; errors.As matches both.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }

func fakeEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func staticDoer(m *mockDoer) func(time.Duration) HTTPDoer {
	return func(time.Duration) HTTPDoer { return m }
}

// --- Pure parsing tests (the mandatory edge cases) ---

func TestParseReviewResponse(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		want       string
		wantErr    bool
		errSubstrs []string
	}{
		{
			name:   "happy path content printed verbatim",
			status: 200,
			body:   `{"choices":[{"message":{"content":"## Review\nLGTM"},"finish_reason":"stop"}]}`,
			want:   "## Review\nLGTM",
		},
		{
			name:   "reasoning fallback when content empty",
			status: 200,
			body:   `{"choices":[{"message":{"content":"","reasoning_content":"reasoned review"},"finish_reason":"stop"}]}`,
			want:   "reasoned review",
		},
		{
			name:       "empty content with finish_reason length",
			status:     200,
			body:       `{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`,
			wantErr:    true,
			errSubstrs: []string{"finish_reason: length"},
		},
		{
			name:       "non-200 includes status code and body snippet",
			status:     400,
			body:       `{"error":{"message":"max_tokens too large"}}`,
			wantErr:    true,
			errSubstrs: []string{"400", "max_tokens too large"},
		},
		{
			name:       "200 error envelope with no choices",
			status:     200,
			body:       `{"error":{"message":"context length exceeded"}}`,
			wantErr:    true,
			errSubstrs: []string{"context length exceeded"},
		},
		{
			name:       "unparseable body",
			status:     200,
			body:       `not json at all`,
			wantErr:    true,
			errSubstrs: []string{"unparseable"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseReviewResponse(tc.status, []byte(tc.body))
			if tc.wantErr {
				require.Error(t, err)
				for _, s := range tc.errSubstrs {
					assert.Contains(t, err.Error(), s)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildChatURL(t *testing.T) {
	const want = "https://host/v1/openai/compat/chat/completions"
	assert.Equal(t, want, buildChatURL("https://host/v1/openai/compat"))
	assert.Equal(t, want, buildChatURL("https://host/v1/openai/compat/"))
}

// --- requestReview tests (request shape + transport errors) ---

func TestRequestReviewBuildsRequest(t *testing.T) {
	m := &mockDoer{status: 200, resp: `{"choices":[{"message":{"content":"REVIEW TEXT"}}]}`}
	cfg := reviewConfig{
		model: "qwen3", baseURL: "https://host/v1", apiKey: "secret",
		systemPrompt: "sys prompt", maxTokens: 4096,
	}

	got, err := requestReview(m, cfg, "THE COMPOSITE")
	require.NoError(t, err)
	assert.Equal(t, "REVIEW TEXT", got)

	require.True(t, m.called)
	assert.Equal(t, http.MethodPost, m.gotReq.Method)
	assert.Equal(t, "https://host/v1/chat/completions", m.gotReq.URL.String())
	assert.Equal(t, "Bearer secret", m.gotReq.Header.Get("Authorization"))
	assert.Equal(t, "application/json", m.gotReq.Header.Get("Content-Type"))

	var sent chatRequest
	require.NoError(t, json.Unmarshal(m.gotBody, &sent))
	assert.Equal(t, "qwen3", sent.Model)
	assert.Equal(t, 4096, sent.MaxTokens)
	require.Len(t, sent.Messages, 2)
	assert.Equal(t, "system", sent.Messages[0].Role)
	assert.Equal(t, "sys prompt", sent.Messages[0].Content)
	assert.Equal(t, "user", sent.Messages[1].Role)
	assert.Equal(t, "THE COMPOSITE", sent.Messages[1].Content)
}

func TestRequestReviewTimeoutIsDistinct(t *testing.T) {
	m := &mockDoer{err: timeoutErr{}}
	cfg := reviewConfig{model: "M", baseURL: "https://host/v1", apiKey: "k", timeout: 5 * time.Second}

	_, err := requestReview(m, cfg, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.NotContains(t, err.Error(), "connection error")
}

func TestRequestReviewConnectionError(t *testing.T) {
	m := &mockDoer{err: errors.New("dial tcp: connection refused")}
	cfg := reviewConfig{model: "M", baseURL: "https://host/v1", apiKey: "k"}

	_, err := requestReview(m, cfg, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection error")
}

// --- runReview end-to-end tests (over a fixture repo, no network) ---

func TestRunReviewHappyPath(t *testing.T) {
	root, _, baseRef := setupSinceFixtureRepo(t)
	m := &mockDoer{status: 200, resp: `{"choices":[{"message":{"content":"FINAL REVIEW"}}]}`}
	var stdout, stderr bytes.Buffer

	code := runReview(
		[]string{"--since", baseRef, "--model", "M", "--base-url", "https://host/v1", root},
		fakeEnv(map[string]string{"DEWDROPS_API_KEY": "k"}),
		&stdout, &stderr, staticDoer(m),
	)

	assert.Equal(t, 0, code)
	assert.Equal(t, "FINAL REVIEW", stdout.String()) // verbatim, no extra newline
	require.True(t, m.called)

	// The composite (with the diff) must reach the user message.
	var sent chatRequest
	require.NoError(t, json.Unmarshal(m.gotBody, &sent))
	assert.Contains(t, sent.Messages[1].Content, "## Diff")
	assert.Contains(t, sent.Messages[1].Content, "func NewFunc(x int) error")
}

func TestRunReviewMissingAPIKeyExits2NoCall(t *testing.T) {
	root, _, baseRef := setupSinceFixtureRepo(t)
	m := &mockDoer{status: 200, resp: `{"choices":[{"message":{"content":"x"}}]}`}
	var stdout, stderr bytes.Buffer

	code := runReview(
		[]string{"--since", baseRef, "--model", "M", "--base-url", "https://host/v1", root},
		fakeEnv(map[string]string{}), // no DEWDROPS_API_KEY
		&stdout, &stderr, staticDoer(m),
	)

	assert.Equal(t, 2, code)
	assert.False(t, m.called, "no HTTP call should be made when the key is missing")
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "DEWDROPS_API_KEY")
}

func TestRunReviewNoChangesExits0(t *testing.T) {
	root, _, _ := setupSinceFixtureRepo(t)
	m := &mockDoer{status: 200, resp: `{"choices":[{"message":{"content":"x"}}]}`}
	var stdout, stderr bytes.Buffer

	code := runReview(
		[]string{"--since", "HEAD", "--model", "M", "--base-url", "https://host/v1", root},
		fakeEnv(map[string]string{"DEWDROPS_API_KEY": "k"}),
		&stdout, &stderr, staticDoer(m),
	)

	assert.Equal(t, 0, code)
	assert.Empty(t, stdout.String())
	assert.False(t, m.called, "no HTTP call when there is nothing to review")
}

func TestRunReviewBadPathExits1(t *testing.T) {
	m := &mockDoer{}
	var stdout, stderr bytes.Buffer

	code := runReview(
		[]string{"--since", "HEAD", "--model", "M", "--base-url", "https://host/v1", "/no/such/dir/xyz"},
		fakeEnv(map[string]string{"DEWDROPS_API_KEY": "k"}),
		&stdout, &stderr, staticDoer(m),
	)

	assert.Equal(t, 1, code)
	assert.False(t, m.called)
	assert.Contains(t, stderr.String(), "not a valid directory")
}

func TestRunReviewHelpExits0(t *testing.T) {
	m := &mockDoer{}
	var stdout, stderr bytes.Buffer

	code := runReview([]string{"-h"}, fakeEnv(nil), &stdout, &stderr, staticDoer(m))

	assert.Equal(t, 0, code)
	assert.False(t, m.called)
}

func TestRunReviewPromptFileOverrides(t *testing.T) {
	root, _, baseRef := setupSinceFixtureRepo(t)
	promptPath := filepath.Join(root, "myprompt.txt")
	require.NoError(t, os.WriteFile(promptPath, []byte("CUSTOM SYSTEM PROMPT"), 0644))

	m := &mockDoer{status: 200, resp: `{"choices":[{"message":{"content":"ok"}}]}`}
	var stdout, stderr bytes.Buffer

	code := runReview(
		[]string{"--since", baseRef, "--model", "M", "--base-url", "https://host/v1", "--prompt-file", promptPath, root},
		fakeEnv(map[string]string{"DEWDROPS_API_KEY": "k"}),
		&stdout, &stderr, staticDoer(m),
	)

	require.Equal(t, 0, code)
	var sent chatRequest
	require.NoError(t, json.Unmarshal(m.gotBody, &sent))
	assert.Equal(t, "CUSTOM SYSTEM PROMPT", sent.Messages[0].Content)
}

func TestRunReviewDefaultPromptUsed(t *testing.T) {
	root, _, baseRef := setupSinceFixtureRepo(t)
	m := &mockDoer{status: 200, resp: `{"choices":[{"message":{"content":"ok"}}]}`}
	var stdout, stderr bytes.Buffer

	code := runReview(
		[]string{"--since", baseRef, "--model", "M", "--base-url", "https://host/v1", root},
		fakeEnv(map[string]string{"DEWDROPS_API_KEY": "k"}),
		&stdout, &stderr, staticDoer(m),
	)

	require.Equal(t, 0, code)
	var sent chatRequest
	require.NoError(t, json.Unmarshal(m.gotBody, &sent))
	assert.Equal(t, defaultReviewPrompt, sent.Messages[0].Content)
}

// model + base-url may come from env for convenience (flag > env > default).
func TestRunReviewEnvFallbackForModelAndBaseURL(t *testing.T) {
	root, _, baseRef := setupSinceFixtureRepo(t)
	m := &mockDoer{status: 200, resp: `{"choices":[{"message":{"content":"ok"}}]}`}
	var stdout, stderr bytes.Buffer

	code := runReview(
		[]string{"--since", baseRef, root},
		fakeEnv(map[string]string{
			"DEWDROPS_API_KEY":  "k",
			"DEWDROPS_MODEL":    "envmodel",
			"DEWDROPS_BASE_URL": "https://envhost/v1",
		}),
		&stdout, &stderr, staticDoer(m),
	)

	require.Equal(t, 0, code)
	assert.Equal(t, "https://envhost/v1/chat/completions", m.gotReq.URL.String())
	var sent chatRequest
	require.NoError(t, json.Unmarshal(m.gotBody, &sent))
	assert.Equal(t, "envmodel", sent.Model)
}
