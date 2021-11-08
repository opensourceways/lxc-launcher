package cmd

import (
	"errors"
	"fmt"
	"github.com/opensourceways/lxc-launcher/log"
	"github.com/opensourceways/lxc-launcher/lxd"
	"github.com/opensourceways/lxc-launcher/network"
	"github.com/opensourceways/lxc-launcher/task"
	"github.com/opensourceways/lxc-launcher/util"
	"github.com/urfave/cli/v2"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"net/http"
	"time"
)

var (
	instName     string
	lxcImage     string
	lxdClient    *lxd.Client
	networkProxy *network.Proxy
	prober       *task.Prober
	statusServer *http.Server
)

const (
	InstanceContainer  = "container"
	InstanceVM         = "virtual-machine"
	NetworkMaxWaitTime = 120
)

const (
	LXDSocket        = "lxd-socket"
	LXDServerAddress = "lxd-server-address"
	ClientKeyPath    = "client-key-path"
	ClientCertPath   = "client-cert-path"
	InstanceType     = "instance-type"
	InstanceProfiles = "instance-profiles"
	CPUResource      = "cpu-resource"
	MemoryResource   = "memory-resource"
	StoragePool      = "storage-pool"
	RootSize         = "root-size"
	NetworkIngress   = "network-ingress"
	NetworkEgress    = "network-egress"
	ProxyPort        = "proxy-port"
	DeviceName       = "device-name"
	InstanceEnvs     = "instance-envs"
	StartCommand     = "start-command"
	MountFiles       = "mount-files"
	ExposePort       = "expose-port"
	AdditionalConfig = "additional-config"
	RemoveExisting   = "remove-existing"
	StatusPort       = "status-port"
	ImageAlias       = "image-alias"
)

var launchCommand = &cli.Command{
	Name:    "launch",
	Aliases: []string{"l"},
	Usage:   "Launch a lxc instance with specification: launcher launch <instance-name> <image-name>",
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
			Name:    InstanceType,
			Aliases: []string{"t"},
			Value:   "container",
			Usage:   "instance type container or virtual machine, default is container",
			EnvVars: []string{GenerateEnvFlags(InstanceType)},
		},
		&cli.StringSliceFlag{
			Name:    InstanceProfiles,
			Aliases: []string{"p"},
			Usage:   "profiles will be applied to instances",
			EnvVars: []string{GenerateEnvFlags(InstanceProfiles)},
		},
		&cli.StringFlag{
			Name:    CPUResource,
			Aliases: []string{"rc"},
			Value:   "",
			Usage:   "CPU limitation of lxc instance",
			EnvVars: []string{GenerateEnvFlags(CPUResource)},
		},
		&cli.StringFlag{
			Name:    MemoryResource,
			Aliases: []string{"rm"},
			Value:   "",
			Usage:   "Memory limitation of lxc instance",
			EnvVars: []string{GenerateEnvFlags(MemoryResource)},
		},
		&cli.StringFlag{
			Name:     StoragePool,
			Aliases:  []string{"sp"},
			Required: true,
			Value:    "",
			Usage:    "Storage pool for lxc instance",
			EnvVars:  []string{GenerateEnvFlags(StoragePool)},
		},
		&cli.StringFlag{
			Name:     RootSize,
			Aliases:  []string{"rd"},
			Required: true,
			Value:    "",
			Usage:    "Root size for lxc instance",
			EnvVars:  []string{GenerateEnvFlags(RootSize)},
		},
		&cli.StringFlag{
			Name:    NetworkIngress,
			Aliases: []string{"ri"},
			Value:   "",
			Usage:   "Ingress limit for lxc instance",
			EnvVars: []string{GenerateEnvFlags(NetworkIngress)},
		},
		&cli.StringFlag{
			Name:    NetworkEgress,
			Aliases: []string{"re"},
			Value:   "",
			Usage:   "Egress limit for lxc instance",
			EnvVars: []string{GenerateEnvFlags(NetworkEgress)},
		},
		&cli.Int64Flag{
			Name:    ProxyPort,
			Aliases: []string{"pp"},
			Value:   0,
			Usage:   "Proxy port, used to forward requests to lxc instance, for example: tcp:<ip-address>:80, empty means no forwarding",
			EnvVars: []string{GenerateEnvFlags(ProxyPort)},
		},
		&cli.StringFlag{
			Name:    DeviceName,
			Aliases: []string{"dn"},
			Value:   "eth0",
			Usage:   "default network device name, can be used for request forwarding",
			EnvVars: []string{GenerateEnvFlags(DeviceName)},
		},
		&cli.StringSliceFlag{
			Name:    InstanceEnvs,
			Aliases: []string{"e"},
			Usage:   "Instance environment, for example: ENV=production.",
			EnvVars: []string{GenerateEnvFlags(InstanceEnvs)},
		},
		&cli.StringFlag{
			Name:    StartCommand,
			Aliases: []string{"sc"},
			Value:   "",
			Usage:   "Instance startup command (non-interactive & short-term), for example: systemctl start nginx. command will be wrapped as 'sh -c <command_input>'",
			EnvVars: []string{GenerateEnvFlags(StartCommand)},
		},
		&cli.StringSliceFlag{
			Name:    MountFiles,
			Aliases: []string{"m"},
			Usage:   "Mount files into instance in the format of <source>:<destination>",
			EnvVars: []string{GenerateEnvFlags(MountFiles)},
		},
		&cli.Int64Flag{
			Name:    ExposePort,
			Aliases: []string{"ep"},
			Value:   8080,
			Usage:   "Expose port for lxc proxy address",
			EnvVars: []string{GenerateEnvFlags(ExposePort)},
		},
		&cli.StringSliceFlag{
			Name:    AdditionalConfig,
			Aliases: []string{"ac"},
			Usage:   "Additional config for lxd instance, in the format of `--additional-config key=value`",
			EnvVars: []string{GenerateEnvFlags(AdditionalConfig)},
		},
		&cli.BoolFlag{
			Name:    RemoveExisting,
			Aliases: []string{"rem"},
			Value:   true,
			Usage:   "Whether to remove existing lxc instance",
			EnvVars: []string{GenerateEnvFlags(RemoveExisting)},
		},
		&cli.Int64Flag{
			Name:    StatusPort,
			Aliases: []string{"stp"},
			Value:   8082,
			Usage:   "health server port",
			EnvVars: []string{GenerateEnvFlags(StatusPort)},
		},
		&cli.StringFlag{
			Name:     ImageAlias,
			Aliases:  []string{"im"},
			Value:    "",
			Required: true,
			Usage:    "image alias for lxd instance",
			EnvVars:  []string{GenerateEnvFlags(ImageAlias)},
		},
	},
	Before: validateLaunch,
	Action: handleLaunch,
}

func validateLaunch(c *cli.Context) error {
	var err error
	if c.Args().Len() < 1 {
		return errors.New("require instance name")
	}
	instName = c.Args().Get(0)
	lxcImage = c.String(ImageAlias)
	if (len(c.String(LXDSocket)) == 0 || !fileutil.Exist(c.String(LXDSocket))) && len(c.String(LXDServerAddress)) == 0 {
		return errors.New(fmt.Sprintf("lxd socket file %s not existed and lxd server address %s not specified",
			c.String(LXDSocket), c.String(LXDServerAddress)))
	}

	if c.String(InstanceType) != InstanceVM && c.String(InstanceType) != InstanceContainer {
		return errors.New("lxd only accepts virtual machine or container type")
	}

	if lxdClient, err = lxd.NewClient(c.String(LXDSocket), c.String(LXDServerAddress), c.String(ClientKeyPath),
		c.String(ClientCertPath), log.Logger); err != nil {
		return err
	}

	log.Logger.Info(fmt.Sprintf("start to validate resource limit on instance %s", instName))
	if err = lxdClient.ValidateResourceLimit(
		c.String(NetworkEgress), c.String(NetworkIngress), c.String(RootSize),
		c.String(StoragePool), c.String(MemoryResource), c.String(CPUResource), c.StringSlice(AdditionalConfig),
		c.String(DeviceName)); err != nil {
		return err
	}
	log.Logger.Info(fmt.Sprintf("start to check image %s existence", lxcImage))
	imageExists, err := lxdClient.CheckImageByAlias(lxcImage)
	if err != nil {
		return err
	}
	if !imageExists {
		return errors.New(fmt.Sprintf("unable to find image by alias %s", lxcImage))
	}
	log.Logger.Info(fmt.Sprintf("start to check instance %s existence", instName))
	instanceExists, err := lxdClient.CheckInstanceExists(instName, c.String(InstanceType))
	if err != nil {
		return err
	}
	log.Logger.Info(fmt.Sprintf("start to check storage pool %s existence", c.String(StoragePool)))
	if len(c.String(StoragePool)) == 0 {
		return errors.New("please specify storage pool name")
	}
	if existed, err := lxdClient.CheckPoolExists(c.String(StoragePool)); err != nil || !existed {
		if err != nil {
			return err
		}
		return errors.New(fmt.Sprintf("storage pool %s not existed", c.String(StoragePool)))
	}
	if instanceExists && c.Bool(RemoveExisting) {
		log.Logger.Info(fmt.Sprintf("start to remove lxc instance %s due to existence", instName))
		err = lxdClient.StopInstance(instName, true)
		if err != nil {
			return err
		}
	}

	return nil
}

func createInstance(c *cli.Context) error {
	instanceExists, err := lxdClient.CheckInstanceExists(instName, c.String(InstanceType))
	if err != nil {
		return err
	}
	if !instanceExists {
		//launch instance
		log.Logger.Info(fmt.Sprintf("start to create instance %s", instName))
		err = lxdClient.CreateInstance(lxcImage, instName, c.StringSlice(InstanceProfiles), c.String(InstanceType))
		if err != nil {
			return err
		}
	}
	log.Logger.Info(fmt.Sprintf("start to launch instance %s", instName))
	err = lxdClient.LaunchInstance(instName, c.StringSlice(InstanceEnvs), c.String(StartCommand), c.String(DeviceName), NetworkMaxWaitTime)
	if err != nil {
		return err
	}
	return nil
}

func CleanupLaunch() {
	if networkProxy != nil {
		networkProxy.Close()
	}
	if prober != nil {
		prober.Close()
	}
	if statusServer != nil {
		statusServer.Close()
	}
	if len(instName) != 0 && lxdClient != nil {
		if err := lxdClient.StopInstance(instName, true); err != nil {
			log.Logger.Error(fmt.Sprintf("failed to clean up lxd instance %s, %s", instName, err))
		}
	}
	time.Sleep(10 * time.Second)
}

func launchStatusHandler(w http.ResponseWriter, req *http.Request) {
	if prober.Alive() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("instance %s alive", instName)))
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("instance %s go dead", instName)))
	}
}

func handleLaunch(c *cli.Context) error {
	var err error
	var ipaddress string
	// create and wait instance ready
	if err = createInstance(c); err != nil {
		return err
	}
	//start proxy if needed
	if c.Int64(ProxyPort) != 0 {
		ipaddress, err = lxdClient.WaitInstanceNetworkReady(instName, c.String(DeviceName), NetworkMaxWaitTime)
		if err != nil {
			CleanupLaunch()
			return err
		}
		networkProxy, err = network.NewProxy(instName, "0.0.0.0", c.Int64(ExposePort),
			fmt.Sprintf("%s:%d", ipaddress, c.Int64(ProxyPort)), log.Logger)
		if err != nil {
			CleanupLaunch()
			return err
		}
		//watch instance status
		prober, err = task.NewProber(instName, lxdClient, 5, log.Logger)
		if err != nil {
			CleanupLaunch()
			return err
		}
		// start health status
		go prober.StartLoop()
		go util.ServerHealth(launchStatusHandler, c.Int64(StatusPort))
		//watch os signal
		util.ListenSignals(CleanupLaunch)
		//start proxying
		networkProxy.StartLoop()
	}
	return nil
}
