package devdocs

import (
	"context"
	"strings"
	"testing"
)

func TestSubcommandHelpSucceeds(t *testing.T) {
	for _, command := range []string{"config", "init", "list", "attach", "status", "sync", "link", "unlink"} {
		code, _, errOut := runCLI(context.Background(), "", command, "--help")
		if code != 0 || strings.Contains(errOut, "error:") {
			t.Errorf("%s --help: code %d, stderr %q", command, code, errOut)
		}
	}
}
