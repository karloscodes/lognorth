package http

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/mcp"
	"lognorth/internal/settings"
)

// MCPAction handles POST /mcp
// Streamable HTTP transport for the Model Context Protocol, authenticated with
// the same agent key as the agent read API.
func MCPAction(ctx *cartridge.Context) error {
	if !agentAuthorized(ctx) {
		return nil
	}

	var req mcp.Request
	if err := json.Unmarshal(ctx.Ctx.Body(), &req); err != nil {
		return ctx.Ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"jsonrpc": "2.0",
			"id":      nil,
			"error":   fiber.Map{"code": -32700, "message": "invalid JSON"},
		})
	}

	res, answered := mcp.Handle(ctx.DB(), req)
	if !answered {
		// A notification. The spec wants an acknowledgement with no body, and
		// SendStatus would fill the body with the status text.
		ctx.Ctx.Status(fiber.StatusAccepted)
		return nil
	}

	return ctx.Ctx.JSON(res)
}

// agentAuthorized checks the agent API key in the Authorization header. It
// writes the 401 itself and reports false, so the caller must stop.
//
// It returns a bool rather than an error on purpose: fiber's Status().JSON()
// returns nil when the write succeeds, so an `err != nil` check here would
// never fire and the handler would serve the data under a 401.
func agentAuthorized(ctx *cartridge.Context) bool {
	authHeader := ctx.Ctx.Get("Authorization")
	key := strings.TrimPrefix(authHeader, "Bearer ")

	if key == "" {
		ctx.Ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "API key required",
		})
		return false
	}

	if err := settings.ValidateAgentKey(ctx.DB(), key); err != nil {
		ctx.Ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid agent key",
		})
		return false
	}

	return true
}
