// Package explain finds public events near a busy day, so a spike can be
// matched to a Hacker News post or a release instead of looking suspicious.
package explain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultHNEndpoint is the free Hacker News search API run by Algolia.
const DefaultHNEndpoint = "https://hn.algolia.com/api/v1/search"

// Post is a Hacker News story that links to the repo.
type Post struct {
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Points    int       `json:"points"`
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

// Link returns the discussion page of the post.
func (p Post) Link() string { return "https://news.ycombinator.com/item?id=" + p.ID }

// HN searches Hacker News.
type HN struct {
	Endpoint string
	HTTP     *http.Client
}

// Posts returns stories whose link points at the repo (or at its homepage,
// when one is given), with at least minPoints points.
func (h HN) Posts(ctx context.Context, repo, homepage string, minPoints int) ([]Post, error) {
	targets := []string{"github.com/" + strings.ToLower(repo)}
	if host := homepageHost(homepage); host != "" {
		targets = append(targets, host)
	}
	seen := map[string]bool{}
	var posts []Post
	for _, target := range targets {
		found, err := h.search(ctx, target)
		if err != nil {
			return posts, err
		}
		for _, post := range found {
			if seen[post.ID] || post.Points < minPoints || !strings.Contains(strings.ToLower(post.URL), target) {
				continue
			}
			seen[post.ID] = true
			posts = append(posts, post)
		}
	}
	return posts, nil
}

func (h HN) search(ctx context.Context, target string) ([]Post, error) {
	endpoint := h.Endpoint
	if endpoint == "" {
		endpoint = DefaultHNEndpoint
	}
	params := url.Values{}
	params.Set("query", target)
	params.Set("restrictSearchableAttributes", "url")
	params.Set("tags", "story")
	params.Set("hitsPerPage", "100")
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "startrust")
	client := h.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Hacker News search: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the Hacker News search returned HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var answer struct {
		Hits []struct {
			Title     string `json:"title"`
			URL       string `json:"url"`
			Points    int    `json:"points"`
			CreatedAt int64  `json:"created_at_i"`
			ObjectID  string `json:"objectID"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, fmt.Errorf("unexpected answer from Hacker News search: %w", err)
	}
	posts := make([]Post, 0, len(answer.Hits))
	for _, hit := range answer.Hits {
		posts = append(posts, Post{
			Title:     hit.Title,
			URL:       hit.URL,
			Points:    hit.Points,
			CreatedAt: time.Unix(hit.CreatedAt, 0).UTC(),
			ID:        hit.ObjectID,
		})
	}
	return posts, nil
}

// sharedHosts serve many projects, so a link to them says nothing about
// this repo.
var sharedHosts = []string{
	"github.com", "github.io", "gitlab.com", "npmjs.com", "pypi.org", "crates.io",
	"pkg.go.dev", "readthedocs.io", "medium.com", "twitter.com", "x.com", "youtube.com",
}

// homepageHost returns the host and path of a project homepage, unless it
// is a generic host that many projects share.
func homepageHost(homepage string) string {
	u, err := url.Parse(strings.TrimSpace(homepage))
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	// A github.io site is one project's page, so its path also counts.
	if strings.HasSuffix(host, ".github.io") {
		return host + strings.TrimSuffix(strings.ToLower(u.Path), "/")
	}
	for _, shared := range sharedHosts {
		if host == shared || strings.HasSuffix(host, "."+shared) {
			return ""
		}
	}
	return host
}
