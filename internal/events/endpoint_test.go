package events

import (
	"fmt"
	"testing"
	"time"
)

func TestEndpointKey(t *testing.T) {
	for _, c := range []struct {
		name, route, path, want string
	}{
		{"a path without ids is its own key", "", "/api/checkout", "/api/checkout"},
		{"digits become :id", "", "/users/123", "/users/:id"},
		{"each id segment becomes :id", "", "/users/123/orders/9", "/users/:id/orders/:id"},
		{"a UUID becomes :id", "", "/orders/3f2c1a9e-8b7d-4c6e-9f0a-1b2c3d4e5f60", "/orders/:id"},
		{"long hex becomes :id", "", "/objects/64b7f3e2a91c4d0012345678", "/objects/:id"},
		{"short hex stays", "", "/colors/beef", "/colors/beef"},
		{"a slug stays without a route", "", "/posts/hello-world", "/posts/hello-world"},
		{"the root stays", "", "/", "/"},
		{"an Express route that covers the path", "/posts/:slug", "/posts/hello-world", "/posts/:slug"},
		{"a Go pattern with the method and braces", "GET /users/{id}", "/users/123", "/users/:id"},
		{"a Go pattern with a host", "GET example.com/items/{sku}", "/items/AB-12", "/items/:sku"},
		{"a Go wildcard to the end", "/files/{path...}", "/files/a/b/c.txt", "/files/*path"},
		{"a Go subtree pattern", "/static/", "/static/css/app.css", "/static/"},
		{"a Go exact-root pattern", "GET /{$}", "/", "/"},
		{"a router-relative Express route does not cover the path", "/:id", "/api/users/7", "/api/users/:id"},
		{"an Express route under a router mounted on an id", "/orgs/42/posts/:slug", "/orgs/42/posts/hello", "/orgs/:id/posts/:slug"},
		{"a route with another literal does not cover the path", "/posts/:slug", "/pages/about", "/pages/about"},
		{"no path, no key", "/x", "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := EndpointKey(c.route, c.path); got != c.want {
				t.Errorf("EndpointKey(%q, %q) = %q, want %q", c.route, c.path, got, c.want)
			}
		})
	}
}

func TestEndpointOf(t *testing.T) {
	db := setupTestDB(t)
	at := time.Now().UTC().Truncate(5 * time.Minute)
	for _, key := range []string{"/users/:id", "/posts/:slug", "/api/checkout"} {
		db.Create(&EndpointStat{APIKeyID: 1, Path: key, Bucket: at, Total: 10})
	}
	db.Create(&EndpointStat{APIKeyID: 2, Path: "/posts/:slug", Bucket: at, Total: 10})

	for _, c := range []struct{ path, want string }{
		{"/api/checkout", "/api/checkout"},
		{"/users/123", "/users/:id"},
		{"/users/:id", "/users/:id"},
		{"/posts/hello-world", "/posts/:slug"},
		{"/nowhere", ""},
	} {
		t.Run(fmt.Sprintf("finds the endpoint of %s", c.path), func(t *testing.T) {
			got, err := EndpointOf(db, 1, c.path)

			if err != nil || got != c.want {
				t.Errorf("EndpointOf(%q) = %q, %v; want %q", c.path, got, err, c.want)
			}
		})
	}

	t.Run("names every app with traffic on the path", func(t *testing.T) {
		apps, err := AppsWithPath(db, "/posts/a-post")

		if err != nil || len(apps) != 2 {
			t.Errorf("AppsWithPath = %v, %v; want apps 1 and 2", apps, err)
		}
	})
}

func TestStatBatchKeysOnTheEndpoint(t *testing.T) {
	db := setupTestDB(t)
	bucket := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	batch := NewStatBatch()

	for id := 1; id <= 1000; id++ {
		batch.Add(db, 1, EndpointKey("", fmt.Sprintf("/users/%d", id)), id%2 == 0, 10, bucket)
	}
	batch.Flush(db)

	var rows []EndpointStat
	db.Find(&rows)
	if len(rows) != 1 || rows[0].Path != "/users/:id" || rows[0].Total != 1000 || rows[0].Errors != 500 {
		t.Errorf("rows = %+v, want one /users/:id row with 1000 requests and 500 errors", rows)
	}
}

func TestListByEndpoint(t *testing.T) {
	db := setupTestDB(t)
	for _, path := range []string{"/users/1", "/users/22", "/users/3/orders", "/users", "/posts/hello", "/posts/a/b", "/files/a/b/c"} {
		Create(db, &Event{APIKeyID: 1, Message: "GET " + path, Context: `{"path":"` + path + `"}`})
	}

	for _, c := range []struct {
		endpoint string
		want     int
	}{
		{"/users/:id", 2},
		{"/users", 1},
		{"/users/:id/orders", 1},
		{"/posts/:slug", 1},
		{"/files/*path", 1},
		{"/nowhere/:id", 0},
	} {
		t.Run(c.endpoint, func(t *testing.T) {
			found, total, err := List(db, ListFilters{APIKeyID: 1, Endpoint: c.endpoint})

			if err != nil || total != int64(c.want) || len(found) != c.want {
				t.Errorf("found %d (total %d, err %v), want %d", len(found), total, err, c.want)
			}
		})
	}

	// The Endpoints page and north open an endpoint by searching for its key.
	t.Run("a search for an endpoint with a parameter reads as the endpoint", func(t *testing.T) {
		found, total, err := List(db, ListFilters{APIKeyID: 1, Search: "/users/:id"})

		if err != nil || total != 2 || len(found) != 2 {
			t.Errorf("found %d (total %d, err %v), want /users/1 and /users/22", len(found), total, err)
		}
	})

	t.Run("a search for a plain path stays a text search", func(t *testing.T) {
		found, _, _ := List(db, ListFilters{APIKeyID: 1, Search: "/users"})

		if len(found) != 4 {
			t.Errorf("found %d, want the 4 events that mention /users", len(found))
		}
	})
}
