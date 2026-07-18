package version_test

import (
	"testing"

	"github.com/Spoloborota/selftracked/internal/version"
)

func TestStringCarriesTheVersion(t *testing.T) {
	t.Parallel()

	got := version.String()
	if want := "selftracked " + version.Version; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
