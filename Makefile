.PHONY: help test run

help:
	@printf 'Available targets:\n'
	@printf '  make test  Run Go tests\n'
	@printf '  make run   Run the ICS MCP server locally\n'

test:
	go test ./...

run:
	go run main.go serve

