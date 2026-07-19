package cli

import "flag"

// FlagSetForTest exposes the registry's flag wiring so the structural
// --json test can enumerate what every registered sub actually carries.
func FlagSetForTest(v *Verb, s *Sub, jsonOut *bool) *flag.FlagSet {
	return flagSet(v.Name, s.Name, s, jsonOut)
}

// ClassifyForTest exposes the §6.1 mapper for direct assertion.
func ClassifyForTest(err error) (int, string) { return classify(err) }
