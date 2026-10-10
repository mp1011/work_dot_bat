// Package work_dot_bat holds the assets compiled into the binary: the vendored scripts that
// `sync` installs into target repos and the templates `bootstrap` renders.
package work_dot_bat

import "embed"

// Vendor is src/vendor/: the manifest and the files it versions, laid out as in a target repo.
//
//go:embed all:src/vendor
var Vendor embed.FS

// Templates is templates/files/: the agent docs, instructions and settings a new repo gets.
//
//go:embed all:templates/files
var Templates embed.FS
