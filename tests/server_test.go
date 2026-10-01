package tests

import (
	"testing"

	"github.com/karloscodes/cartridge"
	"github.com/karloscodes/cartridge/testsupport"

	lbhttp "lognorth/internal/http"
)

// apiRoute is how the app mounts routes for non-browser clients: no CSRF check.
var apiRoute = &cartridge.RouteConfig{EnableSecFetchSite: cartridge.Bool(false)}

// newTestServer mounts the real handlers on a cartridge server, in memory.
func newTestServer(t *testing.T, dbManager cartridge.DBManager, mount func(*cartridge.Server)) *cartridge.Server {
	t.Helper()

	cfg := cartridge.DefaultServerConfig()
	cfg.Config = testsupport.NewTestConfig()
	cfg.Logger = testsupport.NewTestLogger()
	cfg.DBManager = dbManager
	server, err := cartridge.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	mount(server)

	return server
}

// mountIngest mounts the real ingest handler, as the app does.
func mountIngest(server *cartridge.Server) {
	server.Post("/api/v1/events", lbhttp.IngestAction, apiRoute)
}
