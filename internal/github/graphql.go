package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Stargazer is one account that starred a repo, with the profile facts
// StarTrust compares across spikes.
type Stargazer struct {
	Login       string    `json:"login"`
	StarredAt   time.Time `json:"starred_at"`
	CreatedAt   time.Time `json:"created_at"`
	Followers   int       `json:"followers"`
	PublicRepos int       `json:"public_repos"`
}

// ErrStargazersHidden means GitHub returned an empty stargazer list for a
// repo that has stars. GitHub does this without an error when the token
// cannot see who starred (since July 2026, only people with push access can).
var ErrStargazersHidden = errors.New("GitHub does not show who starred this repo to your token; only people with push access can see that")

const stargazerQuery = `query($owner: String!, $name: String!, $after: String) {
  repository(owner: $owner, name: $name) {
    stargazerCount
    stargazers(first: 100, after: $after, orderBy: {field: STARRED_AT, direction: ASC}) {
      pageInfo { hasNextPage endCursor }
      edges {
        starredAt
        node {
          login
          createdAt
          followers { totalCount }
          repositories(privacy: PUBLIC, ownerAffiliations: OWNER) { totalCount }
        }
      }
    }
  }
}`

// Stargazers lists who starred a repo, oldest first, 100 per GraphQL query.
// It needs a token with push access to the repo. Pass the cursor from an
// earlier, interrupted run to continue where it stopped.
//
// onBatch is called after each query with the accounts so far and the cursor
// to resume from. When a query fails, the accounts read so far are returned
// with the error.
func (c *Client) Stargazers(ctx context.Context, fullName, cursor string, onBatch func(all []Stargazer, cursor string)) ([]Stargazer, error) {
	if c.Token == "" {
		return nil, ErrStargazersHidden
	}
	owner, name, _ := strings.Cut(fullName, "/")
	var all []Stargazer
	for {
		variables := map[string]any{"owner": owner, "name": name, "after": nil}
		if cursor != "" {
			variables["after"] = cursor
		}
		var data struct {
			Repository *struct {
				StargazerCount int `json:"stargazerCount"`
				Stargazers     struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Edges []struct {
						StarredAt time.Time `json:"starredAt"`
						Node      struct {
							Login     string    `json:"login"`
							CreatedAt time.Time `json:"createdAt"`
							Followers struct {
								TotalCount int `json:"totalCount"`
							} `json:"followers"`
							Repositories struct {
								TotalCount int `json:"totalCount"`
							} `json:"repositories"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"stargazers"`
			} `json:"repository"`
		}
		if err := c.graphQL(ctx, stargazerQuery, variables, &data); err != nil {
			return all, err
		}
		if data.Repository == nil {
			return all, &Error{Kind: ErrNotFound, Path: "/graphql"}
		}
		stargazers := data.Repository.Stargazers
		// When the token may not see stargazers, GitHub answers with an empty
		// list and no error. Comparing with the star count shows that.
		if len(all) == 0 && cursor == "" && len(stargazers.Edges) == 0 && data.Repository.StargazerCount > 0 {
			return nil, ErrStargazersHidden
		}
		for _, edge := range stargazers.Edges {
			all = append(all, Stargazer{
				Login:       edge.Node.Login,
				StarredAt:   edge.StarredAt,
				CreatedAt:   edge.Node.CreatedAt,
				Followers:   edge.Node.Followers.TotalCount,
				PublicRepos: edge.Node.Repositories.TotalCount,
			})
		}
		cursor = stargazers.PageInfo.EndCursor
		if onBatch != nil {
			onBatch(all, cursor)
		}
		if !stargazers.PageInfo.HasNextPage || cursor == "" {
			return all, nil
		}
	}
}

func (c *Client) graphQL(ctx context.Context, query string, variables map[string]any, dst any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	res, err := c.do(ctx, "POST", "/graphql", payload)
	if err != nil {
		return err
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(res.body, &envelope); err != nil {
		return badResponse("/graphql", err)
	}
	if len(envelope.Errors) > 0 {
		first := envelope.Errors[0]
		kind := ErrUpstream
		switch first.Type {
		case "NOT_FOUND":
			kind = ErrNotFound
		case "RATE_LIMITED":
			kind = ErrRateLimited
		case "FORBIDDEN":
			kind = ErrForbidden
		}
		return &Error{Kind: kind, Status: 200, Path: "/graphql", Message: c.redact(first.Message)}
	}
	if err := json.Unmarshal(envelope.Data, dst); err != nil {
		return badResponse("/graphql", fmt.Errorf("data: %w", err))
	}
	return nil
}
