package cli

import (
	"errors"

	sqlite "modernc.org/sqlite"
)

// The §6.1 exit contract, in one mapper: 0 success · 1 refusal (understood
// and correctly denied — every SQLITE_CONSTRAINT-family failure, and the
// integrity refusals verbs express as CodedError{Status:1}) · 2 environment or
// infrastructure (busy, corrupt, usage). The constraint-vs-infrastructure
// split leans on the driver's EXTENDED result codes, which S1c proved
// distinguishable; the fallback for a non-driver error is classification
// by type, and an error nobody classified is infrastructure — an
// unexplained failure must not masquerade as a clean refusal.
const (
	exitRefusal = 1
	exitInfra   = 2

	// SQLite primary codes (rescode.html). The extended code's low byte is
	// its primary family.
	sqlitePrimaryMask = 0xff
	sqliteConstraint  = 19
	sqliteBusy        = 5
	sqliteLocked      = 6
	sqliteCorrupt     = 11
	sqliteNotADB      = 26
)

// classify maps a verb error to (exit status, error code string).
func classify(err error) (int, string) {
	var coded *CodedError
	if errors.As(err, &coded) {
		return coded.Status, coded.Code
	}
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() & sqlitePrimaryMask {
		case sqliteConstraint:
			return exitRefusal, "constraint"
		case sqliteBusy, sqliteLocked:
			return exitInfra, "busy"
		case sqliteCorrupt, sqliteNotADB:
			return exitInfra, "corrupt"
		}
		return exitInfra, "sqlite"
	}
	return exitInfra, "error"
}
