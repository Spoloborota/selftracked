// Command selftracked is the CLI: one binary, a closed verb set, --json
// everywhere. The same main serves the strk alias — the name is a second
// build artifact, not a second behaviour.
package main

import (
	"os"

	"github.com/Spoloborota/selftracked/internal/cli"
)

func main() {
	os.Exit(run())
}

// run exists so the testscript harness can execute the real entrypoint
// in-process; os.Exit lives one frame up.
func run() int {
	reg := &cli.Registry{}
	// The catalog's verbs register here as their stages build them (S5a
	// onward). Until then the closed set is empty and every verb token is
	// a usage refusal — which is the §1 contract, not a gap: an agent
	// never gets a verb that does not fully exist.
	env := &cli.Env{Stdout: os.Stdout, Stderr: os.Stderr}
	return cli.Run(reg, env, os.Args[1:])
}
