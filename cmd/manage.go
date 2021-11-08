package cmd

import (
	"errors"
	"fmt"
	"github.com/opensourceways/lxc-launcher/image"
	"github.com/opensourceways/lxc-launcher/log"
	"github.com/opensourceways/lxc-launcher/lxd"
	"github.com/opensourceways/lxc-launcher/util"
	"github.com/urfave/cli/v2"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"strings"
	"time"
)

var (
	dataFolder   string
	imageHandler *image.Handler
)

const (
	ImageWorker      = "image-worker"
	SyncInterval     = "sync-interval"
	MetaEndpoint     = "meta-endpoint"
	RegistryUser     = "registry-user"
	RegistryPassword = "registry-password"
	ExitWhenUnready  = "exit-when-unready"
)

var manageCommand = &cli.Command{
	Name:    "manage",
	Aliases: []string{"m"},
	Usage:   "Manage lxc images and lxc orphans:launcher manage <data-folder>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    LXDSocket,
			Aliases: []string{"l"},
			Value:   "",
			Usage:   "lxd socket file for communicating",
			EnvVars: []string{strings.Replace(strings.ToUpper(LXDSocket), "-", "_", -1)},
		},
		&cli.StringFlag{
			Name:    LXDServerAddress,
			Aliases: []string{"s"},
			Value:   "",
			Usage:   "lxd server address for communication, only work when lxd socket not specified",
			EnvVars: []string{strings.Replace(strings.ToUpper(LXDServerAddress), "-", "_", -1)},
		},
		&cli.StringFlag{
			Name:    ClientKeyPath,
			Aliases: []string{"k"},
			Value:   "",
			Usage:   "key path for lxd client authentication, only work when lxd socket not specified",
			EnvVars: []string{strings.Replace(strings.ToUpper(ClientKeyPath), "-", "_", -1)},
		},
		&cli.StringFlag{
			Name:    ClientCertPath,
			Aliases: []string{"c"},
			Value:   "",
			Usage:   "cert path for lxd client authentication, only work when lxd socket not specified",
			EnvVars: []string{strings.Replace(strings.ToUpper(ClientCertPath), "-", "_", -1)},
		},
		&cli.StringFlag{
			Name:     StoragePool,
			Aliases:  []string{"sp"},
			Required: true,
			Value:    "",
			Usage:    "Storage pool for lxc instance",
			EnvVars:  []string{strings.Replace(strings.ToUpper(StoragePool), "-", "_", -1)},
		},
		&cli.Int64Flag{
			Name:    ImageWorker,
			Aliases: []string{"w"},
			Value:   4,
			Usage:   "number of sync worker",
			EnvVars: []string{strings.Replace(strings.ToUpper(ImageWorker), "-", "_", -1)},
		},
		&cli.Int64Flag{
			Name:    SyncInterval,
			Aliases: []string{"si"},
			Value:   600,
			Usage:   "interval in seconds between two sync action",
			EnvVars: []string{strings.Replace(strings.ToUpper(SyncInterval), "-", "_", -1)},
		},
		&cli.StringFlag{
			Name:    MetaEndpoint,
			Aliases: []string{"m"},
			Usage:   "endpoint for images metadata",
			EnvVars: []string{strings.Replace(strings.ToUpper(MetaEndpoint), "-", "_", -1)},
		},
		&cli.StringFlag{
			Name:    RegistryUser,
			Aliases: []string{"u"},
			Usage:   "docker registry user",
			EnvVars: []string{strings.Replace(strings.ToUpper(RegistryUser), "-", "_", -1)},
		},
		&cli.StringFlag{
			Name:    RegistryPassword,
			Aliases: []string{"p"},
			Usage:   "docker registry password",
			EnvVars: []string{strings.Replace(strings.ToUpper(ClientKeyPath), "-", "_", -1)},
		},
		&cli.BoolFlag{
			Name:    ExitWhenUnready,
			Aliases: []string{"e"},
			Value:   true,
			Usage:   "exit if lxd server unready",
			EnvVars: []string{strings.Replace(strings.ToUpper(ExitWhenUnready), "-", "_", -1)},
		},
	},
	Before: validateManage,
	Action: startManage,
}

func validateManage(c *cli.Context) error {
	var err error
	if c.Args().Len() < 1 {
		return errors.New("require image sync folder")
	}
	dataFolder = c.Args().First()
	if (len(c.String(LXDSocket)) == 0 || !fileutil.Exist(c.String(LXDSocket))) && len(c.String(LXDServerAddress)) == 0 {
		return errors.New(fmt.Sprintf("lxd socket file %s not existed and lxd server address %s not specified",
			c.String(LXDSocket), c.String(LXDServerAddress)))
	}
	if lxdClient, err = lxd.NewClient(c.String(LXDSocket), c.String(LXDServerAddress),
		c.String(ClientKeyPath), c.String(ClientCertPath), log.Logger); err != nil && c.Bool(ExitWhenUnready) {
		return err
	}
	if c.Bool(ExitWhenUnready) {

	}
	imageHandler, err = image.NewImageHandler(c.String(RegistryUser), c.String(RegistryPassword), dataFolder,
		c.String(MetaEndpoint), c.Int64(ImageWorker), c.Int64(SyncInterval), lxdClient, log.Logger)
	return nil
}

func startManage(c *cli.Context) error {
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
