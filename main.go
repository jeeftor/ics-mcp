package main

import (
	"context"
	"fmt"
	"io"
	"os"
	_ "time/tzdata"

	"github.com/jeeftor/icsmcp/cmd/icsmcp"
)

func main() {
	os.Exit(mainWithExit(os.Args[1:], os.Stdout, os.Stderr))
}

func mainWithExit(args []string, stdout io.Writer, stderr io.Writer) int {
	if err := executeCommand(args, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func executeCommand(args []string, stdout io.Writer, stderr io.Writer) error {
	cmd := icsmcp.NewRootCommand()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd.ExecuteContext(context.Background())
}
