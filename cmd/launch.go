package cmd

import (
	"errors"
	"fmt"
	"github.com/opensourceways/lxc-launcher/log"

	"github.com/opensourceways/lxc-launcher/lxd"
	"github.com/spf13/cobra"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
)

var (
	instName string
	lxcImage string
	cpuResource string
	memoryResource string
	proxyAddress string
	mountFiles []string
	exposePort int32
	removeExisting  bool
	lxdSocket string
	lxdClient *lxd.Client
)


func init() {
	launchCommand.PersistentFlags().StringVar(&lxdSocket, "lxd-socket", "", "lxd socket file for communicating")
	launchCommand.PersistentFlags().StringVar(&cpuResource, "cpu-resource", "", "CPU limitation of lxc instance")
	launchCommand.PersistentFlags().StringVar(&memoryResource, "memory-resource", "", "Memory limitation of lxc instance")
	launchCommand.PersistentFlags().StringVar(&proxyAddress, "proxy-address", "", "Proxy address, used to forward requests to lxc instance. empty means no forwarding")
	launchCommand.PersistentFlags().StringArrayVar(&mountFiles, "mount-files", []string{}, "Mount files into instance in the format of <source>:<destination>")
	launchCommand.PersistentFlags().Int32Var(&exposePort, "expose-port", 8080, "Expose port for lxc proxy address")
	launchCommand.PersistentFlags().BoolVar(&removeExisting, "remove-existing", true, "Whether to remove existing lxc instance")
	launchCommand.MarkPersistentFlagRequired("lxd-socket")
	rootCmd.AddCommand(launchCommand)
}

var launchCommand = &cobra.Command{
	Use:   "launch <instance-name> <image-alias-name>",
	Short: "Launch a lxc instance with specification",
	Long:  `Launch a lxc instance with specification`,
	Args: cobra.ExactArgs(2),
	PreRunE: validateLaunch,
	RunE: handleLaunch,
}

func validateLaunch(cmd *cobra.Command, args []string) error {
	var err error
	if len(args) < 2 {
		return errors.New("require instance name and image alias name")
	}
	instName = args[0]
	lxcImage = args[1]
	if len(lxdSocket) == 0 || !fileutil.Exist(lxdSocket) {
		return errors.New(fmt.Sprintf("lxd socket file %s not existed", lxdSocket))
	}
	if lxdClient, err = lxd.NewClient(lxdSocket, log.Logger); err != nil {
		return err
	}
	log.Logger.Info(fmt.Sprintf("starting to check image %s existence", lxcImage))
	imageExists, err := lxdClient.CheckImageByAlias(lxcImage)
	if err != nil {
		return err
	}
	if !imageExists {
		return errors.New(fmt.Sprintf("unable to find image by alias %s", lxcImage))
	}
	instanceExists, err := lxdClient.CheckInstanceExists(instName, true)
	if err != nil {
		return err
	}
	if instanceExists && !removeExisting {
		return errors.New("conflicted, instance already exists")
	}
	if instanceExists && removeExisting {
		log.Logger.Info(fmt.Sprintf("starting to remove lxc instance %s due to existence", instName))
		err = lxdClient.DeleteInstance(instName)
		if err != nil {
			return err
		}
	}
	return nil
}

func createInstance(cmd *cobra.Command, args []string) error {

	return nil
}

func handleLaunch(cmd *cobra.Command, args []string) error {
	//Steps to launch&proxying lxc instance
	//0. requirement check
	//1. flags validate
	if err := validateLaunch(cmd, args); err != nil {
		return err
	}
	//2. create and wait ready
	//3. create signal handler
	//4. proxying
	if err := createInstance(cmd, args); err != nil {
		return nil
	}

	return nil
}