# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

- Build: `make build`
- Run: `make run`
- Lint: `make lint` (uses golangci-lint with config from .golangci.yml)
- Unit tests: `make unit` (run all tests)
- Single test: `go test -tags=unit -v ./path/to/package -run TestName`
- Vendor dependencies: `make vendor`
- Generate controller code: `make generate`

## Code Style Guidelines

- Go version: 1.20+
- Line length: 200 characters max
- Imports ordering: standard lib → default → github.com/sergelogvinov → k8s.io
- Error handling: Return errors with context using fmt.Errorf("doing X, %w", err)
- Logging: Use structured logging with WithName() and WithValues()
- Code complexity: Keep cyclomatic complexity below 30
- Resource naming: Follow Kubernetes naming conventions
- Types: Use specific types rather than interface{} when possible
- Formatting: Run gofmt -s for code simplification

Always follow existing patterns in similar files, especially for controller implementation.