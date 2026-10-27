// Command startrust checks whether a GitHub repo's stars look bought.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/vincentsch/startrust/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
