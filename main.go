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
	if err := executeCommand(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func executeCommand(args []string, stdout io.Writer, stderr io.Writer) error {
	cmd := icsmcp.NewRootCommand()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd.ExecuteContext(context.Background())
}
