package main

import (
	"context"
	"fmt"
	"os"

	"github.com/oguzhankaracabay/hostshift/internal/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "hostshift: %v\n", err)
		os.Exit(1)
	}
}
