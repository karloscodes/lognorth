package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/karloscodes/cartridge"

	"lognorth/internal/mcp"
	"lognorth/internal/settings"
)

// MCPAction handles POST /mcp
// Streamable HTTP transport for the Model Context Protocol, authenticated with
// the agent key. Tool calls also need a license; internal/mcp checks it.
func MCPAction(ctx *cartridge.Context) error {
	if !agentAuthorized(ctx) {
		return nil
	}

	var req mcp.Request
	if err := json.Unmarshal(ctx.Body(), &req); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(cartridge.Map{
			"jsonrpc": "2.0",
			"id":      nil,
			"error":   cartridge.Map{"code": -32700, "message": "invalid JSON"},
		})
	}

	res, answered := mcp.Handle(ctx.DB(), req)
	if !answered {
		// A notification. The spec wants an acknowledgement with no body, and
		// SendStatus would fill the body with the status text.
		return ctx.Status(http.StatusAccepted).Send(nil)
	}

	return ctx.JSON(res)
}

// agentAuthorized checks the agent API key in the Authorization header. It
// writes the 401 itself and reports false, so the caller must stop.
//
// It returns a bool rather than an error on purpose: Status().JSON()
// returns nil when the write succeeds, so an `err != nil` check here would
// never fire and the handler would serve the data under a 401.
func agentAuthorized(ctx *cartridge.Context) bool {
	authHeader := ctx.Get("Authorization")
	key := strings.TrimPrefix(authHeader, "Bearer ")

	if key == "" {
		ctx.Status(http.StatusUnauthorized).JSON(cartridge.Map{
			"error": "API key required",
		})
		return false
	}

	if err := settings.ValidateAgentKey(ctx.DB(), key); err != nil {
		ctx.Status(http.StatusUnauthorized).JSON(cartridge.Map{
			"error": "Invalid agent key",
		})
		return false
	}

	return true
}
