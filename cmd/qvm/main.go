package main

import (
	"context"
	"fmt"
	"os"

	"github.com/trollixx/qvm/internal/cli"
)

func main() {
	app := cli.NewApp()
	err := app.Run(context.Background(), os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
