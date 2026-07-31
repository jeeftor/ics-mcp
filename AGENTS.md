# Agent Instructions

## MCP Output Contracts
- Use TDD for MCP output shape changes: add or update a failing schema-contract test before changing production code.
- Tests for compact MCP output must assert raw JSON field presence, not only decoded Go values, because omitted false booleans decode the same as explicit false values.
- Cover both direct JSON marshaling and MCP structured content when a field is part of an advertised tool output schema.

## Release Cadence
- Use SemVer deliberately. Patch releases are for compatible bug fixes and tightly scoped polish; do not create a long sequence of patch releases for a stream of distinct features.
- Group related user-facing additions into the next minor release (for example, `v2.7.0` after the calendar/mobile/AI workspace additions). Use a patch only for follow-up fixes to that released feature set.
- Keep unreleased changelog entries while related work is in progress, then tag at a natural verified checkpoint. When the user explicitly asks for a release after a single fix, release that fix as a patch unless it belongs to a broader, unreleased feature group.
