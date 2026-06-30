# Agent Instructions

## MCP Output Contracts
- Use TDD for MCP output shape changes: add or update a failing schema-contract test before changing production code.
- Tests for compact MCP output must assert raw JSON field presence, not only decoded Go values, because omitted false booleans decode the same as explicit false values.
- Cover both direct JSON marshaling and MCP structured content when a field is part of an advertised tool output schema.
