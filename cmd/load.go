package cmd

import (
	"errors"
	"fmt"
	"net"

	"lxc-launcher/image"
	"lxc-launcher/log"
	"lxc-launcher/lxd"

	"github.com/urfave/cli/v2"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
)

const (
	LoadImage = "load-image"
)

var loadCommand = &cli.Command{
	Name:    "load",
	Aliases: []string{"l"},
	Usage:   "Manage lxc images and lxc orphans:launcher manage <data-folder>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    LXDSocket,
			Aliases: []string{"l"},
			Value:   "",
			Usage:   "lxd socket file for communicating",
			EnvVars: []string{GenerateEnvFlags(LXDSocket)},
		},
		&cli.StringFlag{
			Name:    LXDServerAddress,
			Aliases: []string{"s"},
			Value:   "",
			Usage:   "lxd server address for communication, only work when lxd socket not specified",
			EnvVars: []string{GenerateEnvFlags(LXDServerAddress)},
		},
		&cli.StringFlag{
			Name:    ClientKeyPath,
			Aliases: []string{"k"},
			Value:   "",
			Usage:   "key path for lxd client authentication, only work when lxd socket not specified",
			EnvVars: []string{GenerateEnvFlags(ClientKeyPath)},
		},
		&cli.StringFlag{
			Name:    ClientCertPath,
			Aliases: []string{"c"},
			Value:   "",
			Usage:   "cert path for lxd client authentication, only work when lxd socket not specified",
			EnvVars: []string{GenerateEnvFlags(ClientCertPath)},
		},
		&cli.StringFlag{
			Name:    LoadImage,
			Aliases: []string{"load"},
			Value:   "",
			Usage:   "The image needs to be loaded into the LXD instance",
		},
		&cli.StringFlag{
			Name:    RegistryUser,
			Aliases: []string{"u"},
			Value:   "",
			Usage:   "docker registry user",
			EnvVars: []string{GenerateEnvFlags(RegistryUser)},
		},
		&cli.StringFlag{
			Name:    RegistryPassword,
			Aliases: []string{"p"},
			Value:   "",
			Usage:   "docker registry password",
			EnvVars: []string{GenerateEnvFlags(RegistryPassword)},
		},
	},
	Before: validateLoad,
	Action: startLoad,
}

func validateLoad(c *cli.Context) error {
	var err error
	if c.Args().Len() < 1 {
		return errors.New("require image sync folder")
	}
	dataFolder = c.Args().First()
	if (len(c.String(LXDSocket)) == 0 || !fileutil.Exist(c.String(LXDSocket))) && len(c.String(LXDServerAddress)) == 0 {
		return errors.New(fmt.Sprintf("lxd socket file %s not existed and lxd server address %s not specified",
			c.String(LXDSocket), c.String(LXDServerAddress)))
	}
	serverAddress := c.String(LXDServerAddress)
	if net.ParseIP(c.String(LXDServerAddress)) != nil {
		serverAddress = fmt.Sprintf("https://%s:8443", c.String(LXDServerAddress))
	}
	if len(c.String(LoadImage)) == 0 {
		return errors.New("the lod image is required")
	}
	if lxdClient, err = lxd.NewClient(c.String(LXDSocket), serverAddress, c.String(ClientKeyPath), c.String(ClientCertPath), log.Logger); err != nil {
		log.Logger.Info(fmt.Sprintln("lxd.NewClient, err: ", err))
		return err
	}

	imageHandler, err = image.NewImageHandler(c.String(RegistryUser), c.String(RegistryPassword), dataFolder,
		c.String(MetaEndpoint), c.Int64(ImageWorker), c.Int64(SyncInterval), lxdClient, log.Logger)
	return nil
}

func startLoad(c *cli.Context) error {
	loadImage := c.String(LoadImage)

	err := image.LoadImage(loadImage, imageHandler)
	if err != nil {
		log.Logger.Error(fmt.Sprintf("load image err: %s", err.Error()))
		return err
	}

	return nil
}
