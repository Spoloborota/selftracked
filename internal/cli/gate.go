package cli

// versionGate is the §8.6 gate every verb begins with (§6.1). At S2 it is
// deliberately a stub with the final call shape: the dispatcher already
// routes every verb through it, so when S11 implements the real gate —
// compare the database's schema version to the binary's, refuse or
// escalate into a migration — no verb's control flow changes, only this
// function's body does. A stub that exists and is called is a different
// thing from a gate that is absent: the pipeline order is already load-
// bearing and testable.
func versionGate() error {
	return nil
}
