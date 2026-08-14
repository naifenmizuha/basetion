package main

import (
	"context"
	"os"

	"github.com/naifenmizuha/basetion/src/internal/bootstrap"
)

func main() {
	os.Exit(bootstrap.Execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
