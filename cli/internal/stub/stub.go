// Package stub provides standardized "not implemented in M1" handlers for
// CLI subcommands whose real bodies arrive in later milestones.
package stub

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NotImplementedYet returns a RunE that prints a clear deferral notice and
// exits non-zero. The milestone tag (e.g. "M3") tells users when the
// behavior is planned.
func NotImplementedYet(milestone string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		return fmt.Errorf(
			"%s is not implemented in M1 (planned for %s); see docs/design/2026-05-02-loom-design.md § 17",
			cmd.Name(), milestone)
	}
}
