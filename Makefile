.PHONY: help test run ui-install ui-build ui-test browser-smoke

help:
	@printf 'Available targets:\n'
	@printf '  make test  Run Go tests\n'
	@printf '  make run   Run the ICS MCP server locally\n'
	@printf '  make ui-build  Build the embedded React admin UI\n'
	@printf '  make ui-test   Run frontend tests\n'
	@printf '  make browser-smoke  Run desktop and mobile browser smoke coverage\n'

ui-install:
	pnpm install --frozen-lockfile

ui-build: ui-install
	pnpm --filter icsmcp-admin-ui run build

ui-test: ui-install
	pnpm --filter icsmcp-admin-ui run test

browser-smoke:
	./scripts/browser-smoke.sh

test: ui-build ui-test
	go test ./...

run: ui-build
	go run main.go serve
