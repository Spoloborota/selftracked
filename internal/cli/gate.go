package cli

// VersionGate is the §8.6 gate every verb begins with (§6.1). The
// dispatcher routes every verb through it before Run; main wires it to
// the migrate package's CLIGate — the machinery sits above cli in the
// import graph (migrate → load → verb → cli), so the dispatcher holds
// the call site and main supplies the body. A nil hook gates nothing:
// that is the wiring-less test registry's state, never the shipped
// binary's (cmd/selftracked assigns it before the first dispatch, and
// TestRunWiresTheVersionGate holds that claim).
var VersionGate func(e *Env, verbName string) error

func versionGate(e *Env, verbName string) error {
	if VersionGate == nil {
		return nil
	}
	return VersionGate(e, verbName)
}
