package explain

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostsMatchTheRepoURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("restrictSearchableAttributes") != "url" {
			t.Errorf("search must look at story URLs only")
		}
		io.WriteString(w, `{"hits":[
			{"title":"Glow: Markdown in the terminal","url":"https://github.com/charmbracelet/glow","points":283,"created_at_i":1602900000,"objectID":"1"},
			{"title":"Glowing bugs","url":"https://example.com/glow","points":500,"created_at_i":1602900000,"objectID":"2"},
			{"title":"Tiny post","url":"https://github.com/charmbracelet/glow/issues/1","points":3,"created_at_i":1602900000,"objectID":"3"}
		]}`)
	}))
	defer server.Close()
	posts, err := HN{Endpoint: server.URL}.Posts(context.Background(), "charmbracelet/glow", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || posts[0].Points != 283 || posts[0].Link() != "https://news.ycombinator.com/item?id=1" {
		t.Errorf("posts = %+v", posts)
	}
}

func TestHomepageHost(t *testing.T) {
	cases := map[string]string{
		"https://charm.sh":                "charm.sh",
		"https://www.example.dev/docs":    "example.dev",
		"https://github.com/a/b":          "",
		"https://someone.github.io/tool/": "someone.github.io/tool",
		"":                                "",
	}
	for in, want := range cases {
		if got := homepageHost(in); got != want {
			t.Errorf("homepageHost(%q) = %q, want %q", in, got, want)
		}
	}
}
