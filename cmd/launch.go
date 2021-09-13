package cmd

import (
	"errors"
	"fmt"
	"github.com/opensourceways/lxc-launcher/log"
	"github.com/opensourceways/lxc-launcher/lxd"
	"github.com/opensourceways/lxc-launcher/network"
	"github.com/opensourceways/lxc-launcher/task"
	"github.com/spf13/cobra"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	instName         string
	lxcImage         string
	cpuResource      string
	memoryResource   string
	storagePool      string
	rootSize         string
	ingressLimit     string
	egressLimit      string
	proxyEndpoint    string
	instEnvs         []string
	startCommand     string
	mountFiles       []string
	exposePort       int32
	removeExisting   bool
	instSocketDir    string
	instSocketFile   string
	lxdSocket        string
	lxdClient        *lxd.Client
	additionalConfig []string
	networkProxy     *network.Proxy
	prober           *task.Prober
	statusPort       int32
	statusServer     *http.Server
)

func init() {
	launchCommand.PersistentFlags().StringVar(&lxdSocket, "lxd-socket", "", "lxd socket file for communicating")
	launchCommand.PersistentFlags().StringVar(&instSocketDir, "instance-socket-dir", "",
		"Directory for holding instance socket file, ensure this folder exist and access both on host and container")
	launchCommand.PersistentFlags().StringVar(&cpuResource, "cpu-resource", "", "CPU limitation of lxc instance")
	launchCommand.PersistentFlags().StringVar(&memoryResource, "memory-resource", "", "Memory limitation of lxc instance")
	launchCommand.PersistentFlags().StringVar(&storagePool, "storage-pool", "", "Storage pool for lxc instance")
	launchCommand.PersistentFlags().StringVar(&rootSize, "root-size", "", "Root size for lxc instance")
	launchCommand.PersistentFlags().StringVar(&ingressLimit, "network-ingress", "", "Ingress limit for lxc instance")
	launchCommand.PersistentFlags().StringVar(&egressLimit, "network-egress", "", "Egress limit for lxc instance")
	launchCommand.PersistentFlags().StringVar(&proxyEndpoint, "proxy-endpoint", "", "Proxy endpoint, used to forward requests to lxc instance, for example: tcp:127.0.0.1:80, empty means no forwarding")
	launchCommand.PersistentFlags().StringArrayVar(&instEnvs, "instance-envs", []string{}, "Instance environment, for example: ENV=production.")
	launchCommand.PersistentFlags().StringVar(&startCommand, "start-command", "", "Instance startup command (non-interactive & short-term), for example: systemctl start nginx.")
	launchCommand.PersistentFlags().StringArrayVar(&mountFiles, "mount-files", []string{}, "Mount files into instance in the format of <source>:<destination>")
	launchCommand.PersistentFlags().Int32Var(&exposePort, "expose-port", 8080, "Expose port for lxc proxy address")
	launchCommand.PersistentFlags().StringArrayVar(&additionalConfig, "additional-config", []string{}, "Additional config for lxd instance, in the format of `--additional-config key=value`")
	launchCommand.PersistentFlags().BoolVar(&removeExisting, "remove-existing", true, "Whether to remove existing lxc instance")
	launchCommand.PersistentFlags().Int32Var(&statusPort, "status-port", 8082, "health server port")
	launchCommand.MarkPersistentFlagRequired("lxd-socket")
	launchCommand.MarkPersistentFlagRequired("storage-pool")
	rootCmd.AddCommand(launchCommand)
}

var launchCommand = &cobra.Command{
	Use:     "launch <instance-name> <image-alias-name>",
	Short:   "Launch a lxc instance with specification",
	Long:    `Launch a lxc instance with specification`,
	Args:    cobra.ExactArgs(2),
	PreRunE: validateLaunch,
	RunE:    handleLaunch,
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
	//if len(instSocketDir)!= 0 && !fileutil.Exist(instSocketDir) {
	//	return errors.New(fmt.Sprintf("instance socket file directory %s not existed", instSocketDir))
	//}
	if len(instSocketDir) != 0 {
		instSocketFile = fmt.Sprintf("%s/%s.sock", strings.TrimRight(instSocketDir, "/"), instName)
	}

	if lxdClient, err = lxd.NewClient(lxdSocket, log.Logger); err != nil {
		return err
	}

	log.Logger.Info(fmt.Sprintf("start to validate resource limit on instance %s", instName))
	if err = lxdClient.ValidateResourceLimit(
		egressLimit, ingressLimit, rootSize, storagePool, memoryResource, cpuResource, additionalConfig); err != nil {
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
	instanceExists, err := lxdClient.CheckInstanceExists(instName, true)
	if err != nil {
		return err
	}
	log.Logger.Info(fmt.Sprintf("start to check storage pool %s existence", storagePool))
	if len(storagePool) == 0 {
		return errors.New("please specify storage pool name")
	}
	if existed, err := lxdClient.CheckPoolExists(storagePool); err != nil || !existed {
		if err != nil {
			return err
		}
		return errors.New(fmt.Sprintf("storage pool %s not existed", storagePool))
	}
	if instanceExists && removeExisting {
		log.Logger.Info(fmt.Sprintf("start to remove lxc instance %s due to existence", instName))
		err = lxdClient.StopInstance(instName, true)
		if err != nil {
			return err
		}
	}

	return nil
}

func createInstance(cmd *cobra.Command, args []string) error {
	instanceExists, err := lxdClient.CheckInstanceExists(instName, true)
	if err != nil {
		return err
	}
	if !instanceExists {
		//launch instance
		log.Logger.Info(fmt.Sprintf("start to create instance %s", instName))
		err = lxdClient.CreateInstance(lxcImage, instName)
		if err != nil {
			return err
		}
	}
	log.Logger.Info(fmt.Sprintf("start to launch instance %s", instName))
	err = lxdClient.LaunchInstance(instName, instSocketFile, proxyEndpoint, instEnvs, startCommand)
	if err != nil {
		return err
	}
	return nil
}

func Cleanup() {
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
}

func handleLaunch(cmd *cobra.Command, args []string) error {
	var err error
	// create and wait instance ready
	if err := createInstance(cmd, args); err != nil {
		return err
	}
	//start proxy if needed
	if len(instSocketFile) != 0 {
		networkProxy, err = network.NewProxy(instName, "0.0.0.0", exposePort, instSocketFile, log.Logger)
		if err != nil {
			Cleanup()
			return err
		}
		//watch instance status
		prober, err = task.NewProber(instName, lxdClient, 5, log.Logger)
		if err != nil {
			Cleanup()
			return err
		}
		// start health status
		go prober.StartLoop()
		go serverHealth()
		//watch os signal
		listenSignals()
		//start proxying
		networkProxy.StartLoop()
	}
	return nil
}

func statusHandler(w http.ResponseWriter, req *http.Request) {
	if prober.Alive() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("instance %s alive", instName)))
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("instance %s go dead", instName)))
	}
}

func serverHealth() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", statusHandler)
	statusServer := http.Server{
		Addr:           fmt.Sprintf("0.0.0.0:%d", statusPort),
		Handler:        mux,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	err := statusServer.ListenAndServe()
	if err != nil {
		log.Logger.Error(fmt.Sprintf("failed to setup status server %v", err))
	}
}

// listenSignals Graceful start/stop server
func listenSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go handleSignals(sigChan)
}

// handleSignals handle process signal
func handleSignals(c chan os.Signal) {
	log.Logger.Info("Notice: System signal monitoring is enabled(watch: SIGINT,SIGTERM,SIGQUIT)\n")

	switch <-c {
	case syscall.SIGINT:
		log.Logger.Info("\nShutdown by Ctrl+C")
	case syscall.SIGTERM:
		log.Logger.Info("\nShutdown quickly")
	case syscall.SIGQUIT:
		log.Logger.Info("\nShutdown gracefully")
	}

	//kill proxy process and kill lxd instance
	Cleanup()
	os.Exit(0)
}
