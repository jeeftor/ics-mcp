package main

import (
	"context"
	"fmt"
	"os"
	_ "time/tzdata"

	"github.com/jeeftor/icsmcp/cmd/icsmcp"
)

func main() {
	if err := icsmcp.NewRootCommand().ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
