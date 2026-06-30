package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestMainRunsVersionCommand(t *testing.T) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	t.Cleanup(func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	})

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = writer
	os.Stderr = writer

	code := mainWithExit([]string{"version"}, os.Stdout, os.Stderr)
	if code != 0 {
		t.Fatalf("mainWithExit(version) = %d, want 0", code)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	gotBytes, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	got := string(gotBytes)
	for _, want := range []string{"version:", "commit:", "date:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("main version output missing %q:\n%s", want, got)
		}
	}
}

func TestMainWithExitReturnsZeroForSuccessfulCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := mainWithExit([]string{"version"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("mainWithExit(version) = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "version:") {
		t.Fatalf("stdout = %q, want version output", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestMainWithExitReturnsOneAndPrintsErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := mainWithExit([]string{"does-not-exist"}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("mainWithExit(invalid) = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr = %q, want unknown command", stderr.String())
	}
}

func TestExecuteCommandRunsVersionWithInjectedWriters(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeCommand([]string{"version"}, &stdout, &stderr)

	if err != nil {
		t.Fatalf("executeCommand(version) error = %v", err)
	}
	got := stdout.String()
	for _, want := range []string{"version:", "commit:", "date:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output missing %q:\n%s", want, got)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestExecuteCommandReturnsErrorsWithInjectedWriters(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeCommand([]string{"does-not-exist"}, &stdout, &stderr)

	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("executeCommand(invalid) error = %v, want unknown command", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr = %q, want Cobra error output", stderr.String())
	}
}
