package http

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/karloscodes/cartridge"

	"lognorth/internal/mcp"
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
