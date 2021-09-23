package cmd

import (
	"errors"
	"fmt"
	"github.com/opensourceways/lxc-launcher/image"
	"github.com/opensourceways/lxc-launcher/log"
	"github.com/opensourceways/lxc-launcher/lxd"
	"github.com/opensourceways/lxc-launcher/util"
	"github.com/spf13/cobra"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"time"
)

var (
	dataFolder       string
	imageWorker      int32
	syncInterval     int32
	metaEndpoint     string
	registryUser     string
	registryPassword string
	exitWhenUnReady  bool
	imageHandler     *image.Handler
)

func init() {
	manageCommand.PersistentFlags().StringVar(&lxdSocket, "lxd-socket", "", "lxd socket file for communicating")
	manageCommand.PersistentFlags().Int32Var(&imageWorker, "imageWorker", 4, "number of sync worker")
	manageCommand.PersistentFlags().Int32Var(&syncInterval, "sync-interval", 600, "interval in seconds between two sync action")
	manageCommand.PersistentFlags().StringVar(&metaEndpoint, "meta-endpoint", "", "endpoint for images metadata")
	manageCommand.PersistentFlags().StringVar(&registryUser, "registry-user", "", "docker registry user")
	manageCommand.PersistentFlags().StringVar(&registryPassword, "registry-password", "", "docker registry password")
	manageCommand.PersistentFlags().BoolVar(&exitWhenUnReady, "exit-when-unready", true, "exit if lxd server unready")
	manageCommand.MarkPersistentFlagRequired("lxd-socket")
	manageCommand.MarkPersistentFlagRequired("storage-pool")
	rootCmd.AddCommand(manageCommand)
}

var manageCommand = &cobra.Command{
	Use:     "manage <image-data-folder>",
	Short:   "Manage lxc image and lxc orphan",
	Long:    `Manage lxc image by and lxc orphan`,
	Args:    cobra.ExactArgs(1),
	PreRunE: validateManage,
	RunE:    startManage,
}

func validateManage(cmd *cobra.Command, args []string) error {
	var err error
	if len(args) < 1 {
		return errors.New("require image sync folder")
	}
	dataFolder = args[0]
	if len(lxdSocket) == 0 || !fileutil.Exist(lxdSocket) && exitWhenUnReady {
		return errors.New(fmt.Sprintf("lxd socket file %s not existed", lxdSocket))
	}
	if lxdClient, err = lxd.NewClient(lxdSocket, log.Logger); err != nil && exitWhenUnReady {
		return err
	}
	if exitWhenUnReady {

	}
	imageHandler, err = image.NewImageHandler(registryUser, registryPassword, dataFolder,
		metaEndpoint, int(imageWorker), syncInterval, lxdClient, log.Logger)
	return nil
}

func startManage(cmd *cobra.Command, args []string) error {
	//watch os signal
	util.ListenSignals(CleanupManage)
	if lxdClient == nil {
		//It's not guaranteed we do have lxd server on all node. we can fail to sleep infinitely in case of this.
		imageHandler.FakeLoop()
	} else {
		imageHandler.StartLoop()
	}
	return nil
}

func CleanupManage() {
	if imageHandler != nil {
		imageHandler.Close()
	}
	time.Sleep(6 * time.Second)
}
