package main

import (
	_ "embed"

	"github.com/DevOpsForEveryone/gha/cmd"
	"github.com/DevOpsForEveryone/gha/pkg/common"
)

//go:embed VERSION
var version string

func main() {
	ctx, cancel := common.CreateGracefulJobCancellationContext()
	defer cancel()

	// run the command
	cmd.Execute(ctx, version)
}
