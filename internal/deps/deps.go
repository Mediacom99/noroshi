// Package deps exists solely to anchor external dependencies in go.mod.
// Remove this file once each package imports its own dependencies directly.
package deps

import (
	_ "github.com/go-co-op/gocron/v2"
	_ "github.com/hashicorp/go-retryablehttp"
	_ "github.com/pressly/goose/v3"
	_ "gopkg.in/telebot.v4"
	_ "modernc.org/sqlite"
)
