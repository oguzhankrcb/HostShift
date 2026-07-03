package main

import (
	"context"
	"fmt"
	"os"

	"github.com/oguzhankaracabay/hostshift/internal/cli"
)

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "mcp" && os.Args[2] == "stdio" {
		if err := cli.ServeMCP(context.Background(), os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "hostshift mcp: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "hostshift: %v\n", err)
		os.Exit(1)
	}
}
