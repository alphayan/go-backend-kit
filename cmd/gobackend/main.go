package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alphayan/go-backend-kit/internal/cli"
)

var (
	version = "devel"
	commit  = ""
	date    = ""
)

func main() {
	if err := cli.Execute(context.Background(), cli.BuildInfo{Version: version, Commit: commit, Date: date}, os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gobackend:", err)
		os.Exit(1)
	}
}
