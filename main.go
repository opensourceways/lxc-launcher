package main

import (
	"github.com/urfave/cli/v2"
	"log"
	"lxc-launcher/cmd"
	"os"
	"sort"
)

func main() {
	sort.Sort(cli.FlagsByName(cmd.RootCmd.Flags))
	sort.Sort(cli.CommandsByName(cmd.RootCmd.Commands))
	err := cmd.RootCmd.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
