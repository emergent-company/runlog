# Web UI Tests

Web UI handler tests live in `cmd/runlog/*_test.go` (with `package main`)
because Go cannot import `package main` code from an external package.

This directory is reserved for future black-box integration tests that
start the daemon binary and test via HTTP client, once the binary is built.
