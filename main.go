package main

import (
	_ "embed"

	"github.com/Leapfrog-DevOps/gha/cmd"
	"github.com/Leapfrog-DevOps/gha/pkg/common"
)

//go:embed VERSION
var version string

func main() {
	ctx, cancel := common.CreateGracefulJobCancellationContext()
	defer cancel()

	// run the command
	cmd.Execute(ctx, version)
}
