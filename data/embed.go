// Package data embeds first-party datasets used by the runtime image.
package data

import _ "embed"

//go:embed poi/beijing.json
var BeijingPOI []byte
