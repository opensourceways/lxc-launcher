package cmd

import (
	"fmt"
	"github.com/opensourceways/lxc-launcher/log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile       string
	lxdSocketFile string

	rootCmd = &cobra.Command{
		Use:   "lxc-launcher",
		Short: "A tool for lxc instance management & proxy tool",
		Long: `Lxc launcher acts as a lxc instance agent in kubernetes which 
responsible for lxc instance lifecycle management as well as the network proxy`,
	}
)

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "$HOME/.lxc-launcher.yaml", "config file")
	rootCmd.PersistentFlags().StringVar(&lxdSocketFile, "lxd-socket-file", "/var/lib/lxd/unix.socket",
		"name of socket file to communicate to lxd server")
	viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("lxd-socket-file", rootCmd.PersistentFlags().Lookup("lxd-socket-file"))
}

func initConfig() {
	viper.SetDefault("debug", false)
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".cobra" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".cobra")
	}
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
	log.InitLog(viper.GetBool("debug"))
}