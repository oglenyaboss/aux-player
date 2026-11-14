package cmd

import (
	"errors"
	"fmt"

	"github.com/oglenyaboss/aux-player/internal/updater"
	"github.com/oglenyaboss/aux-player/internal/version"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for a newer release and install it now",
	RunE: func(cmd *cobra.Command, args []string) error {
		res := updater.UpdateNow(version.Version, false, func(msg string) {
			fmt.Println(msg)
		})
		msg, err := updateReport(version.Version, res)
		if err != nil {
			return err
		}
		fmt.Println(msg)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

// updateReport turns an update result into the line aux update prints. With
// noUpdate off, OutcomeDisabled only comes back for a dev build, and Advice
// would wrongly point at --no-update.
func updateReport(current string, res updater.Result) (string, error) {
	switch res.Outcome {
	case updater.OutcomeFailed:
		return "", errors.New("update check failed: could not reach GitHub releases")
	case updater.OutcomeCurrent:
		return fmt.Sprintf("aux %s is the newest release", current), nil
	case updater.OutcomeDisabled:
		return fmt.Sprintf("%s is available, but this is a build from source; pull and rebuild to update", res.Tag), nil
	default:
		return res.Advice(), nil
	}
}
