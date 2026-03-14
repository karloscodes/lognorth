package lognorth

import "embed"

//go:embed web/dist/assets
var StaticFS embed.FS

//go:embed web/dist/.vite/manifest.json
var ManifestData []byte

//go:embed web/public
var PublicFS embed.FS
