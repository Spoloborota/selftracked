// Command selftracked is the CLI: one binary, a closed verb set, --json
// everywhere. The same main serves the strk alias — the name is a second
// build artifact, not a second behaviour.
package main

import (
	"fmt"
	"os"

	"github.com/Spoloborota/selftracked/internal/cli"
	"github.com/Spoloborota/selftracked/internal/dump"
	"github.com/Spoloborota/selftracked/internal/load"
	"github.com/Spoloborota/selftracked/internal/migrate"
	"github.com/Spoloborota/selftracked/internal/scaffold"
	"github.com/Spoloborota/selftracked/internal/verb"
	"github.com/Spoloborota/selftracked/internal/verify"
)

func main() {
	os.Exit(run())
}

// run exists so the testscript harness can execute the real entrypoint
// in-process; os.Exit lives one frame up.
func run() int {
	// §8.6: the version gate the dispatcher runs before every verb. Wired
	// here because the gate's machinery (migrate → load) sits above cli
	// in the import graph.
	cli.VersionGate = migrate.CLIGate

	reg := &cli.Registry{}
	// The catalog's verbs register here as their stages build them; a verb
	// absent from this list does not exist — which is the §1 contract, not
	// a gap: an agent never gets a verb that does not fully work.
	base := []cli.Verb{dump.Verb(), load.Verb(), verify.Verb(), scaffold.Verb()}
	catalog := make([]cli.Verb, 0, len(base)+len(verb.Verbs()))
	catalog = append(catalog, base...)
	catalog = append(catalog, verb.Verbs()...)
	for _, v := range catalog {
		if err := reg.Register(v); err != nil {
			// A registration failure is a build defect, not a runtime
			// condition: infrastructure exit, like any unclassified error.
			fmt.Fprintln(os.Stderr, err)
			const exitInfra = 2
			return exitInfra
		}
	}
	env := &cli.Env{Stdout: os.Stdout, Stderr: os.Stderr}
	return cli.Run(reg, env, os.Args[1:])
}
