package cmd

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// confirmDestroy gates an irreversible delete. With --yes it proceeds silently.
// On an interactive terminal it prompts: when typeName is true (data-destroying
// deletes) the caller must type the resource name; otherwise a y/N suffices.
// When stdin is not a terminal or --no-input is set and --yes was not given, it
// refuses — so agents must opt in explicitly rather than destroy data by default.
func confirmDestroy(kind, name string, typeName bool) error {
	if flagYes {
		return nil
	}
	if flagNoInput || !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("refusing to delete %s %q without confirmation — pass --yes (irreversible)", kind, name)
	}
	if typeName {
		fmt.Fprintf(os.Stderr, "This permanently deletes %s %q and its data. Type the name to confirm: ", kind, name)
		var in string
		fmt.Scanln(&in)
		if in != name {
			return fmt.Errorf("confirmation did not match %q; aborted", name)
		}
		return nil
	}
	fmt.Fprintf(os.Stderr, "Delete %s %q? [y/N]: ", kind, name)
	var in string
	fmt.Scanln(&in)
	if !strings.EqualFold(in, "y") && !strings.EqualFold(in, "yes") {
		return fmt.Errorf("aborted")
	}
	return nil
}
