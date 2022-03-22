package lxd

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	cli "github.com/lxc/lxd/client"
	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"go.uber.org/zap"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"lxc-launcher/common"
	"lxc-launcher/log"
	"lxc-launcher/util"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	STATUS_RUNNING    = "Running"
	ACTION_STOP       = "stop"
	ACTION_START      = "start"
	SOURCE_TYPE_IMAGE = "image"
	STATUS_STOPPED    = "Stopped"
)

const (
	DEL_STOPPED_TIME = 6 * 3600
)

type ResourceLimit struct {
	Device string
	Name   string
	Value  string
}

type Client struct {
	instServer   lxd.InstanceServer
	imageServer  lxd.ImageServer
	logger       *zap.Logger
	DeviceLimits map[string]map[string]string
	Configs      map[string]string
}

func NewClient(socket, server, clientKeyPath, clientSecretPath string, logger *zap.Logger) (*Client, error) {
	var instServer lxd.InstanceServer
	var err error
	if len(socket) != 0 && fileutil.Exist(socket) {
		instServer, err = lxd.ConnectLXDUnix(socket, nil)
	} else {
		if !fileutil.Exist(clientKeyPath) || !fileutil.Exist(clientSecretPath) {
			return nil, errors.New(fmt.Sprintf(
				"client key %s and client secret %s should exist when connect lxd via http",
				clientKeyPath, clientSecretPath))
		}
		keyData, err := ioutil.ReadFile(clientKeyPath)
		if err != nil {
			return nil, err
		}
		certData, err := ioutil.ReadFile(clientSecretPath)
		if err != nil {
			return nil, err
		}
		instServer, err = lxd.ConnectLXD(server, &lxd.ConnectionArgs{
			InsecureSkipVerify: true,
			TLSClientCert:      string(certData),
			TLSClientKey:       string(keyData),
		})
	}
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to connect lxd server via socket file, %s", err))
	}
	return &Client{
		instServer:   instServer,
		logger:       logger,
		Configs:      map[string]string{},
		DeviceLimits: map[string]map[string]string{},
	}, nil
}

func (c *Client) ValidateResourceLimit(egressLimit, ingressLimit, rootSize, storagePool, memoryResource,
	cpuResource string, additionalConfig []string, deviceName string) error {
	//egress limitation
	c.DeviceLimits[deviceName] = map[string]string{}
	if len(egressLimit) != 0 {
		if strings.HasSuffix(egressLimit, "kbit") || strings.HasSuffix(
			egressLimit, "Mbit") || strings.HasSuffix(
			egressLimit, "Gbit") || strings.HasSuffix(egressLimit, "Tbit") {
			c.DeviceLimits[deviceName]["limits.egress"] = egressLimit
		} else if strings.HasSuffix(egressLimit, "k") || strings.HasSuffix(
			egressLimit, "M") || strings.HasSuffix(
			egressLimit, "G") || strings.HasSuffix(egressLimit, "T") {
			c.DeviceLimits[deviceName]["limits.egress"] = fmt.Sprintf("%sbit", egressLimit)
		} else {
			return errors.New(fmt.Sprintf(
				"instance network egress limitation %s incorrect, only support（M|G|T)bit or（M|G|T)", egressLimit))
		}
	}
	//ingress limitation
	if len(ingressLimit) != 0 {
		if strings.HasSuffix(egressLimit, "kbit") || strings.HasSuffix(
			ingressLimit, "Mbit") || strings.HasSuffix(
			ingressLimit, "Gbit") || strings.HasSuffix(ingressLimit, "Tbit") {
			c.DeviceLimits[deviceName]["limits.ingress"] = ingressLimit
		} else if strings.HasSuffix(egressLimit, "k") || strings.HasSuffix(
			ingressLimit, "M") || strings.HasSuffix(
			ingressLimit, "G") || strings.HasSuffix(ingressLimit, "T") {
			c.DeviceLimits[deviceName]["limits.ingress"] = fmt.Sprintf("%sbit", ingressLimit)
		} else {
			return errors.New(fmt.Sprintf(
				"instance network ingress limitation %s incorrect, only support（M|G|T)bit or（M|G|T)", ingressLimit))
		}
	}
	//root size
	c.DeviceLimits["root"] = map[string]string{}
	if len(rootSize) != 0 {
		if strings.HasSuffix(rootSize, "MB") || strings.HasSuffix(
			rootSize, "GB") || strings.HasSuffix(rootSize, "TB") ||
			strings.HasSuffix(rootSize, "MiB") || strings.HasSuffix(
			rootSize, "GiB") || strings.HasSuffix(rootSize, "TiB") {
			c.DeviceLimits["root"]["size"] = rootSize
			c.DeviceLimits["root"]["pool"] = storagePool
			c.DeviceLimits["root"]["type"] = "disk"
			c.DeviceLimits["root"]["path"] = "/"
		} else if strings.HasSuffix(rootSize, "Mi") || strings.HasSuffix(
			rootSize, "Gi") || strings.HasSuffix(rootSize, "Ti") {
			c.DeviceLimits["root"]["size"] = fmt.Sprintf("%sB", rootSize)
			c.DeviceLimits["root"]["pool"] = storagePool
			c.DeviceLimits["root"]["type"] = "disk"
			c.DeviceLimits["root"]["path"] = "/"
		} else {
			return errors.New(fmt.Sprintf(
				"instance storage size limitation %s incorrect, only support（M|G|T)iB or（M|G|T)B", rootSize))
		}
	}
	//memory limitation
	if len(memoryResource) != 0 {
		if strings.HasSuffix(memoryResource, "MB") || strings.HasSuffix(
			memoryResource, "GB") || strings.HasSuffix(memoryResource, "TB") ||
			strings.HasSuffix(memoryResource, "MiB") || strings.HasSuffix(
			memoryResource, "GiB") || strings.HasSuffix(memoryResource, "TiB") {
			c.Configs["limits.memory"] = memoryResource
		} else if strings.HasSuffix(memoryResource, "Mi") || strings.HasSuffix(
			memoryResource, "Gi") || strings.HasSuffix(memoryResource, "Ti") {
			c.Configs["limits.memory"] = fmt.Sprintf("%sB", memoryResource)
		} else {
			return errors.New(fmt.Sprintf(
				"instance memory limitation %s incorrect, only support（M|G|T)iB or（M|G|T)B", memoryResource))
		}
	}
	//cpu limitation
	if len(cpuResource) != 0 {
		if strings.HasSuffix(cpuResource, "%") {
			c.Configs["limits.cpu"] = "1"
			c.Configs["limits.cpu.allowance"] = cpuResource
		} else {
			core, err := strconv.ParseFloat(cpuResource, 64)
			if err != nil {
				return err
			}
			if core >= 1 {
				c.Configs["limits.cpu"] = fmt.Sprintf("%d", int(core))
			} else if core < 1 && core > 0 {
				c.Configs["limits.cpu"] = "1"
				c.Configs["limits.cpu.allowance"] = fmt.Sprintf("%0.0f%%", float64(core)*100)
			} else {
				return errors.New("cpu core must be greater than 0")
			}
		}
	}
	//additional config, for instance: security.nesting=true
	for _, a := range additionalConfig {
		if len(a) != 0 {
			//value may contains equal symbol
			arr := strings.SplitN(a, "=", 2)
			if len(arr) == 2 {
				c.Configs[arr[0]] = arr[1]
			}
		}
	}
	rlimits := "Instance resource limit: "
	for k, v := range c.Configs {
		rlimits += fmt.Sprintf("name:%s,value:%s;", k, v)
	}
	for k, v := range c.DeviceLimits {
		for ik, iv := range v {
			rlimits += fmt.Sprintf("device%s:name:%s,value:%s;", k, ik, iv)
		}
	}
	c.logger.Info(rlimits)
	return nil
}

func (c *Client) CheckPoolExists(name string) (bool, error) {
	names, err := c.instServer.GetStoragePoolNames()
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if name == n {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) ApplyResourceLimit(name string, instEnvs []string) error {
	instance, etag, err := c.instServer.GetInstance(name)
	if err != nil {
		return err
	}
	req := api.InstancePut{
		Config: util.MergeConfigs(instance.Config, c.Configs),
	}
	//add environment if needed
	if len(instEnvs) != 0 {
		for _, e := range instEnvs {
			envs := strings.SplitN(e, "=", 2)
			req.Config[fmt.Sprintf("environment.%s", envs[0])] = envs[1]
		}
	}
	// Use expanded device for resource update
	req.Devices = util.MergeDeviceConfigs(instance.ExpandedDevices, c.DeviceLimits)
	c.logger.Info(fmt.Sprintf("perform instance %s resource limit %v", name, req))
	op, err := c.instServer.UpdateInstance(name, req, etag)
	if err != nil {
		return err
	}
	return op.Wait()
}

func (c *Client) LaunchInstance(name string, instEnvs []string, startCmd string, deviceName string,
	maxWaitTime int32) error {
	instance, etag, err := c.instServer.GetInstance(name)
	if err != nil {
		return err
	}
	if instance.StatusCode == api.Running {
		c.logger.Info(fmt.Sprintf("instance %s already running. will stop it first", name))
		err = c.StopInstance(name, false)
		if err != nil {
			return err
		}
	}
	instance, etag, err = c.instServer.GetInstance(name)
	if instance.StatusCode == api.Error || instance.StatusCode.IsFinal() {
		return errors.New(fmt.Sprintf("instance %s in %s state", name, instance.Status))
	}
	//update instance config
	c.logger.Info(fmt.Sprintf("update instance %s cpu&memory&disk quota", name))
	err = c.ApplyResourceLimit(name, instEnvs)
	if err != nil {
		return err
	}
	c.logger.Info(fmt.Sprintf("start instance %s", name))
	if instance.StatusCode == api.Stopped {
		req := api.InstanceStatePut{
			Action:   ACTION_START,
			Timeout:  -1,
			Force:    true,
			Stateful: false,
		}
		op, err := c.instServer.UpdateInstanceState(name, req, etag)
		if err != nil {
			return err
		}
		err = op.Wait()
		if err != nil {
			return err
		}
	}

	//execute start command
	if len(startCmd) != 0 {
		_, err := c.WaitInstanceNetworkReady(name, deviceName, maxWaitTime)
		command := []string{
			"sh",
			"-c",
			startCmd,
		}
		req := api.InstanceExecPost{
			Command:     command,
			WaitForWS:   true,
			Interactive: false,
			User:        0,
			Group:       0,
		}
		option := lxd.InstanceExecArgs{
			Stdout: os.Stdout,
			Stderr: os.Stdin,
		}
		c.logger.Info(fmt.Sprintf("start to execute command %s on instance %s ", name,
			strings.Join(command, " ")))
		op, err := c.instServer.ExecInstance(name, req, &option)
		if err != nil {
			return err
		}
		return op.Wait()
	}
	return nil
}

func (c *Client) WaitInstanceNetworkReady(name string, network string, maxWaitSeconds int32) (string, error) {
	retry := int32(0)
	for {
		c.logger.Info(fmt.Sprintf("start to get network status %s", name))
		state, err := c.GetInstanceState(name)
		if err != nil {
			return "", err
		}
		for n, device := range state.Network {
			if n != network {
				continue
			}
			if device.State == "up" {
				for _, f := range device.Addresses {
					if f.Family == "inet" {
						return f.Address, nil
					}
				}
			}
		}
		time.Sleep(5 * time.Second)
		retry += 1
		if retry >= maxWaitSeconds/5 {
			return "", errors.New("failed to obtain instance ip address after 2 minutes")
		}
	}
}

func (c *Client) GetInstanceState(name string) (*api.InstanceState, error) {
	c.logger.Info(fmt.Sprintf("start to get instance state %s", name))
	state, _, err := c.instServer.GetInstanceState(name)
	if err != nil {
		return nil, err
	}
	return state, nil
}

func (c *Client) StopInstance(name string, alsoDelete bool) error {
	c.logger.Info(fmt.Sprintf("start to delete instance %s", name))
	instance, etag, err := c.instServer.GetInstance(name)
	if err != nil {
		return err
	}
	if instance.Status == STATUS_RUNNING {
		req := api.InstanceStatePut{
			Action:   ACTION_STOP,
			Timeout:  -1,
			Force:    true,
			Stateful: false,
		}
		op, err := c.instServer.UpdateInstanceState(name, req, etag)
		if err != nil {
			return err
		}
		err = op.Wait()
		if err != nil {
			return err
		}
	}
	if alsoDelete {
		op, err := c.instServer.DeleteInstance(name)
		if err != nil {
			return err
		}
		return op.Wait()
	}
	return nil
}

func (c *Client) CreateInstance(imageAlias string, instanceName string, profiles []string, instType string) error {
	req := api.InstancesPost{
		Name: instanceName,
		Source: api.InstanceSource{
			Type:  SOURCE_TYPE_IMAGE,
			Alias: imageAlias,
		},
		Type: api.InstanceType(instType),
	}
	if _, ok := c.DeviceLimits["root"]; ok {
		req.Devices = map[string]map[string]string{}
		req.Devices["root"] = c.DeviceLimits["root"]
	}
	req.Profiles = profiles
	op, err := c.instServer.CreateInstance(req)
	if err != nil {
		return err
	}
	return op.Wait()
}

func (c *Client) CheckImageByAlias(alias string) (bool, error) {
	aliasNames, err := c.instServer.GetImageAliasNames()
	if err != nil {
		return false, err
	}
	for _, a := range aliasNames {
		if alias == a {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) CheckManageImageByAlias(alias string) (bool, error) {
	aliasValue, _, err := c.instServer.GetImageAlias(alias)
	if err != nil {
		return false, err
	}
	if len(aliasValue.Name) > 0 {
		return true, nil
	}
	return false, nil
}

func (c *Client) DeleteImageAlias(alias string) error {
	delImageErr := c.instServer.DeleteImageAlias(alias)
	if delImageErr != nil {
		log.Logger.Error(fmt.Sprint("delImageErr %s", delImageErr))
		return delImageErr
	}
	return nil
}

func (c *Client) CreateImage(imageApi api.ImagesPost, imageAlias cli.ImageCreateArgs) (op cli.Operation, err error) {
	op, err = c.instServer.CreateImage(imageApi, &imageAlias)
	if err != nil {
		log.Logger.Error(fmt.Sprintf("createImageErr %s", err))
	}
	return
}

func (c *Client) CreateImageAlias(alias api.ImageAliasesPost) (err error) {
	err = c.instServer.CreateImageAlias(alias)
	return
}

func (c *Client) GetImages() (images []api.Image, err error) {
	images, err = c.instServer.GetImages()
	if err != nil {
		log.Logger.Error(fmt.Sprintf("Failed to get the mirror list, err: %v", err))
	}
	return
}

func (c *Client) GetOperation(uuid string) (op *api.Operation, ETag string, err error) {
	aliasValue, ETag, err := c.instServer.GetOperation(uuid)
	if err != nil {
		log.Logger.Error(fmt.Sprintln("alias: ", aliasValue, "ETag:", ETag, "err: ", err))
	}
	return aliasValue, ETag, err
}

func (c *Client) DeleteImage(fingerprint string) (op cli.Operation, err error) {
	op, err = c.instServer.DeleteImage(fingerprint)
	if err != nil {
		log.Logger.Error(fmt.Sprintf("Failed to delete mirror, err: %v", err))
	}
	return
}

func (c *Client) CheckInstanceExists(name string, instanceType string) (bool, error) {
	names, err := c.instServer.GetInstanceNames(api.InstanceType(instanceType))
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if name == n {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) GetInstanceStatus(name string) (string, error) {
	instance, _, err := c.instServer.GetInstance(name)
	if err != nil {
		return "", err
	}
	return instance.Status, nil
}

func (c *Client) DeleteStopInstances(instanceType string) error {
	log.Logger.Info(fmt.Sprintf("--------------------here1------------------------%v",1))
	// 1. Query the status of an existing instance
	instances, err := c.instServer.GetInstances(api.InstanceType(instanceType))
	if err != nil {
		log.Logger.Error(fmt.Sprintf("Query instance failed, err: %v, "+
			"instanceType: %v", err, instanceType))
		return err
	}
	log.Logger.Info(fmt.Sprintf("--------------------here2------------------------"))
	podConf, confErr := GetResConfig("conf")
	if confErr == nil {
		// creates the clientset
		clientset, cliErr := kubernetes.NewForConfig(podConf)
		log.Logger.Error(fmt.Sprintf("cliErr:%v", cliErr))
		// access the API to list pods
		pods, podErr := clientset.CoreV1().Pods("").List(context.TODO(), v1.ListOptions{})
		if podErr == nil {
			for _, pod := range pods.Items {
				fmt.Printf("There are %d pods in the cluster\n, %v", pod.Name, pod)
			}
		} else {
			log.Logger.Error(fmt.Sprintf("podErr:%v", podErr))
		}
	}

	// 2. Perform a delete operation on a stopped instance
	if len(instances) > 0 {
		for _, instance := range instances {
			timeInt := common.TimeStrToInt(instance.LastUsedAt.String()) + 8*3600
			if (common.TimeStrToInt(common.GetCurTime())-timeInt > DEL_STOPPED_TIME) && (instance.Status == STATUS_STOPPED) {
				_, err := c.instServer.DeleteInstance(instance.Name)
				if err != nil {
					log.Logger.Error(fmt.Sprintf("Failed to delete stopped instance, "+
						"err: %v, name: %v", err, instance.Name))
				}
			}
		}
	}
	return nil
}

func CreateDir(dir string) error {
	_, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			os.Mkdir(dir, 0777)
		}
	}
	return err
}

func EncryptMd5(str string) string {
	if str == "" {
		return str
	}
	sum := md5.Sum([]byte(str))
	return fmt.Sprintf("%x", sum)
}

func FileExists(path string) (bool) {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

func DelFile(filex string) {
	if FileExists(filex) {
		err := os.Remove(filex)
		if err != nil {
			log.Logger.Error(fmt.Sprintf("err: %v", err))
		}
	}
}

func GetResConfig(dirPath string) (resConfig *rest.Config, err error) {
	CreateDir(dirPath)
	podConfig := os.Getenv("POD_CONFIG")
	fileName := EncryptMd5(podConfig) + ".json"
	filePath := filepath.Join(dirPath, fileName)
	if FileExists(filePath) {
		DelFile(filePath)
	}
	f, ferr := os.Create(filePath)
	if ferr != nil {
		return resConfig, ferr
	}
	defer DelFile(filePath)
	defer f.Close()
	data, baseErr := base64.StdEncoding.DecodeString(podConfig)
	if baseErr == nil {
		f.Write(data)
	} else {
		return resConfig, baseErr
	}
	resConfig, err = clientcmd.BuildConfigFromFlags("", filePath)
	if err != nil {
		log.Logger.Error(fmt.Sprintf("err: %v", err))
		return
	}
	return
}
