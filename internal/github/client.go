// Package github reads repository data from the GitHub REST and GraphQL APIs.
//
// Everything here is read-only. A token is optional: without one, GitHub
// allows 60 requests an hour, which is enough to check one or two repos.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the public GitHub API.
const DefaultBaseURL = "https://api.github.com"

// apiVersion pins the REST API version so field names stay stable.
const apiVersion = "2022-11-28"

// Client talks to the GitHub API. The zero value is ready to use.
type Client struct {
	BaseURL   string
	Token     string
	HTTP      *http.Client
	UserAgent string

	// MaxRequests stops the client before it sends more requests than this.
	// Zero means no limit.
	MaxRequests int

	// Sleep waits between retries. Tests replace it to run fast.
	Sleep func(context.Context, time.Duration) error

	mu       sync.Mutex
	requests int
	rate     *RateLimit
}

// RateLimit is GitHub's hourly limit for ordinary requests, as one of its
// answers reported it in the X-RateLimit headers.
type RateLimit struct {
	// Limit is how many requests GitHub allows an hour, such as 60 without
	// a token.
	Limit int `json:"limit"`
	// Remaining is how many of them are left.
	Remaining int `json:"remaining"`
	// ResetsAt is when the count starts again from Limit.
	ResetsAt time.Time `json:"resets_at"`
}

// Requests returns how many HTTP requests the client has sent, retries included.
func (c *Client) Requests() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests
}

// RateLimit returns the hourly limit that GitHub sent with its latest
// answer to an ordinary request. It reports false until GitHub sent one.
func (c *Client) RateLimit() (RateLimit, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rate == nil {
		return RateLimit{}, false
	}
	return *c.rate, true
}

func (c *Client) baseURL() string {
	if c.BaseURL == "" {
		return DefaultBaseURL
	}
	return strings.TrimSuffix(c.BaseURL, "/")
}

func (c *Client) httpClient() *http.Client {
	base := http.Client{Timeout: 30 * time.Second}
	if c.HTTP != nil {
		base = *c.HTTP
	}
	// Follow redirects only on the API host. GitHub answers renamed repos
	// with a redirect to /repositories/{id}/..., which is the same host.
	apiHost := hostOf(c.baseURL())
	base.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Host != apiHost {
			return fmt.Errorf("refusing redirect to %s", req.URL.Host)
		}
		return nil
	}
	return &base
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// ErrorKind groups API failures by what the user can do about them.
type ErrorKind int

const (
	ErrUpstream ErrorKind = iota
	ErrNotFound
	ErrAuth
	ErrForbidden
	ErrRateLimited
	ErrNetwork
	ErrBudget
	ErrCanceled
)

// Error is a failed API call with the cause GitHub or the network gave.
type Error struct {
	Kind    ErrorKind
	Status  int
	Message string // GitHub's own message, if any
	Path    string
	ResetAt time.Time
	Err     error
	// WithToken is true when the request carried a token, so the advice to
	// set one would be wrong.
	WithToken bool
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Error() string {
	switch e.Kind {
	case ErrNotFound:
		if name, ok := strings.CutPrefix(e.Path, "/repos/"); ok && ValidName(name) {
			return "GitHub has no public repository called " + name + " (or it is private and your token cannot see it)"
		}
		return "GitHub has no public repository with that name (or it is private and your token cannot see it)"
	case ErrAuth:
		return "GitHub rejected the token (HTTP 401). Check GITHUB_TOKEN or GH_TOKEN, or run `gh auth login`"
	case ErrForbidden:
		msg := "GitHub refused the request (HTTP 403)"
		if e.Message != "" {
			msg += ": " + e.Message
		}
		return msg
	case ErrRateLimited:
		msg := "GitHub's rate limit is used up"
		if e.WithToken {
			msg = "GitHub's rate limit for this token is used up"
		}
		if !e.ResetAt.IsZero() {
			msg += fmt.Sprintf("; it resets at %s", e.ResetAt.Local().Format("15:04"))
		}
		if e.WithToken {
			return msg
		}
		return msg + ". Without a token GitHub allows 60 requests an hour; set GITHUB_TOKEN or run `gh auth login` for 5,000"
	case ErrNetwork:
		return fmt.Sprintf("cannot reach GitHub: %v", e.Err)
	case ErrBudget:
		return "stopped at the request limit (--max-requests)"
	case ErrCanceled:
		return "stopped (Ctrl-C)"
	default:
		msg := fmt.Sprintf("GitHub returned HTTP %d", e.Status)
		if e.Message != "" {
			msg += ": " + e.Message
		}
		if e.Err != nil {
			msg += fmt.Sprintf(" (%v)", e.Err)
		}
		return msg
	}
}

// IsKind reports whether err is a GitHub API error of the given kind.
func IsKind(err error, kind ErrorKind) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Kind == kind
}

var tokenPattern = regexp.MustCompile(`(gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+)`)

// redact removes anything that looks like a GitHub token from text that may
// end up in an error message.
func (c *Client) redact(s string) string {
	if c.Token != "" {
		s = strings.ReplaceAll(s, c.Token, "[redacted]")
	}
	return tokenPattern.ReplaceAllString(s, "[redacted]")
}

// response is what get returns for a successful call.
type response struct {
	header http.Header
	body   []byte
}

// get fetches an API path (or a full URL on the API host, as given in Link
// headers) and retries network errors and server errors twice.
func (c *Client) get(ctx context.Context, target string) (*response, error) {
	return c.do(ctx, http.MethodGet, target, nil)
}

func (c *Client) do(ctx context.Context, method, target string, payload []byte) (*response, error) {
	fullURL := target
	if strings.HasPrefix(target, "/") {
		fullURL = c.baseURL() + target
	} else if hostOf(target) != hostOf(c.baseURL()) {
		return nil, &Error{Kind: ErrUpstream, Path: target, Err: errors.New("link points outside the GitHub API")}
	}
	path := pathOf(fullURL)
	client := c.httpClient()
	sleep := c.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	const attempts = 3
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, &Error{Kind: ErrCanceled, Path: path, Err: err}
		}
		c.mu.Lock()
		if c.MaxRequests > 0 && c.requests >= c.MaxRequests {
			c.mu.Unlock()
			return nil, &Error{Kind: ErrBudget, Path: path}
		}
		c.requests++
		c.mu.Unlock()

		req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(payload))
		if err != nil {
			return nil, &Error{Kind: ErrUpstream, Path: path, Err: err}
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", apiVersion)
		req.Header.Set("User-Agent", c.userAgent())
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}

		res, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, &Error{Kind: ErrCanceled, Path: path, Err: ctx.Err()}
			}
			if attempt < attempts {
				if sleepErr := sleep(ctx, backoff(attempt)); sleepErr != nil {
					return nil, &Error{Kind: ErrCanceled, Path: path, Err: sleepErr}
				}
				continue
			}
			return nil, &Error{Kind: ErrNetwork, Path: path, Err: networkCause(c.redact(err.Error()), err)}
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 32<<20))
		res.Body.Close()
		c.noteRateLimit(res.Header)

		if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusNotModified {
			if readErr != nil {
				return nil, &Error{Kind: ErrNetwork, Path: path, Err: readErr}
			}
			return &response{header: res.Header, body: body}, nil
		}

		apiErr := c.classify(res, body, path)
		retryable := res.StatusCode >= 500 || apiErr.Kind == ErrRateLimited && shortWait(res.Header)
		if retryable && attempt < attempts {
			wait := backoff(attempt)
			if seconds, convErr := strconv.Atoi(res.Header.Get("Retry-After")); convErr == nil && seconds > 0 {
				wait = time.Duration(seconds) * time.Second
			}
			if sleepErr := sleep(ctx, wait); sleepErr != nil {
				return nil, &Error{Kind: ErrCanceled, Path: path, Err: sleepErr}
			}
			continue
		}
		return nil, apiErr
	}
}

func (c *Client) userAgent() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return "startrust"
}

func (c *Client) noteRateLimit(h http.Header) {
	// Search and GraphQL have limits of their own. Only the hourly limit
	// for ordinary requests says how many history pages are left.
	if resource := h.Get("X-RateLimit-Resource"); resource != "" && resource != "core" {
		return
	}
	remaining, err := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	if err != nil {
		return
	}
	rate := RateLimit{Remaining: remaining}
	rate.Limit, _ = strconv.Atoi(h.Get("X-RateLimit-Limit"))
	if reset, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		rate.ResetsAt = time.Unix(reset, 0).UTC()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rate = &rate
}

func (c *Client) classify(res *http.Response, body []byte, path string) *Error {
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &payload)
	apiErr := &Error{Status: res.StatusCode, Message: c.redact(payload.Message), Path: path, WithToken: c.Token != ""}
	if reset, err := strconv.ParseInt(res.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		apiErr.ResetAt = time.Unix(reset, 0)
	}
	lowerMessage := strings.ToLower(payload.Message)
	switch {
	case res.StatusCode == http.StatusNotFound:
		apiErr.Kind = ErrNotFound
	case res.StatusCode == http.StatusUnauthorized:
		apiErr.Kind = ErrAuth
	case res.StatusCode == http.StatusTooManyRequests,
		res.StatusCode == http.StatusForbidden && res.Header.Get("X-RateLimit-Remaining") == "0",
		res.StatusCode == http.StatusForbidden && strings.Contains(lowerMessage, "rate limit"):
		apiErr.Kind = ErrRateLimited
	case res.StatusCode == http.StatusForbidden:
		apiErr.Kind = ErrForbidden
	default:
		apiErr.Kind = ErrUpstream
	}
	return apiErr
}

// shortWait reports whether GitHub asked us to wait at most a minute, which is
// worth doing instead of failing.
func shortWait(h http.Header) bool {
	seconds, err := strconv.Atoi(h.Get("Retry-After"))
	return err == nil && seconds > 0 && seconds <= 60
}

func backoff(attempt int) time.Duration {
	return time.Duration(attempt*attempt) * time.Second
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func pathOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Path
}

// networkCause turns low-level network errors into a short cause a person
// can act on.
func networkCause(text string, err error) error {
	var dnsErr *net.DNSError
	var opErr *net.OpError
	switch {
	case errors.As(err, &dnsErr):
		return fmt.Errorf("DNS lookup failed for %s (are you offline?)", dnsErr.Name)
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(text, "Client.Timeout"):
		return errors.New("the request timed out after 30 seconds")
	case errors.As(err, &opErr) && opErr.Op == "dial":
		return fmt.Errorf("connection failed: %v", opErr.Err)
	case strings.Contains(text, "certificate") || strings.Contains(text, "x509"):
		return errors.New("TLS certificate check failed (a proxy or firewall may be intercepting HTTPS)")
	}
	return errors.New(text)
}

// nextLink returns the rel="next" URL from a Link header, or "" when there is
// no next page. GitHub writes these links with /repositories/{id}/... paths,
// not the /repos/{owner}/{name}/... path we asked for, so the URL is used as
// given. The client only follows links on the API host.
func nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		section := strings.Split(part, ";")
		if len(section) < 2 {
			continue
		}
		target := strings.Trim(strings.TrimSpace(section[0]), "<>")
		for _, attr := range section[1:] {
			if strings.TrimSpace(attr) == `rel="next"` {
				return target
			}
		}
	}
	return ""
}
