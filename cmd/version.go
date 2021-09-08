package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of lxc launcher",
	Long:  `Print the version number of lxc launcher`,
	Run: handleVersion,
}

func handleVersion(cmd *cobra.Command, args []string) {
fmt.Println("Lxc Launcher 0.0.1")
}