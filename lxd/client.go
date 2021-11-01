package lxd

import (
	"errors"
	"fmt"
	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	"github.com/opensourceways/lxc-launcher/util"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"go.uber.org/zap"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	PROXY_DEVICE_NAME = "instance_network_proxy"
	PROXY_SOCKET_MODE = "0666"
	STATUS_RUNNING    = "Running"

	ACTION_STOP       = "stop"
	ACTION_START      = "start"
	SOURCE_TYPE_IMAGE = "image"
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
		keyData, err := os.ReadFile(clientKeyPath)
		if err != nil {
			return nil, err
		}
		certData, err := os.ReadFile(clientSecretPath)
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
	cpuResource string, additionalConfig []string) error {
	//egress limitation
	c.DeviceLimits["eth0"] = map[string]string{}
	if len(egressLimit) != 0 {
		if strings.HasSuffix(egressLimit, "Mbit") || strings.HasSuffix(
			egressLimit, "Gbit") || strings.HasSuffix(egressLimit, "Tbit") {
			c.DeviceLimits["eth0"]["limits.egress"] = egressLimit
		} else {
			return errors.New(fmt.Sprintf("instance network egress limitation %s incorrect", egressLimit))
		}
	}
	//ingress limitation
	if len(ingressLimit) != 0 {
		if strings.HasSuffix(ingressLimit, "Mbit") || strings.HasSuffix(
			ingressLimit, "Gbit") || strings.HasSuffix(ingressLimit, "Tbit") {
			c.DeviceLimits["eth0"]["limits.ingress"] = ingressLimit
		} else {
			return errors.New(fmt.Sprintf("instance network ingress limitation %s incorrect", ingressLimit))
		}
	}
	//root size
	c.DeviceLimits["root"] = map[string]string{}
	if len(rootSize) != 0 {
		if strings.HasSuffix(rootSize, "MB") || strings.HasSuffix(
			rootSize, "GB") || strings.HasSuffix(rootSize, "TB") {
			c.DeviceLimits["root"]["size"] = rootSize
			c.DeviceLimits["root"]["pool"] = storagePool
			c.DeviceLimits["root"]["type"] = "disk"
			c.DeviceLimits["root"]["path"] = "/"
		} else {
			return errors.New(fmt.Sprintf("instance storage size limitation %s incorrect", rootSize))
		}
	}
	//memory limitation
	if len(memoryResource) != 0 {
		if strings.HasSuffix(memoryResource, "MB") || strings.HasSuffix(
			memoryResource, "GB") || strings.HasSuffix(memoryResource, "TB") {
			c.Configs["limits.memory"] = memoryResource
		} else {
			return errors.New(fmt.Sprintf("instance memory limitation %s incorrect", memoryResource))
		}
	}
	//cpu limitation
	if len(cpuResource) != 0 {
		if strings.HasSuffix(cpuResource, "%") {
			c.Configs["limits.cpu"] = "1"
			c.Configs["limits.cpu.allowance"] = cpuResource
		} else {
			core, err := strconv.Atoi(cpuResource)
			if err != nil {
				return err
			}
			if core < 1 {
				return errors.New("cpu core must be equal or greater than 1")
			}
			c.Configs["limits.cpu"] = strconv.Itoa(core)
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
	c.logger.Info(fmt.Sprintf("start to execute command %s on instance %s ", name, startCmd))
	if len(startCmd) != 0 {
		_, err := c.WaitInstanceNetworkReady(name, deviceName, maxWaitTime)
		command := strings.Split(startCmd, " ")
		fmt.Println(command)
		req := api.InstanceExecPost{
			Command:     command,
			WaitForWS:   false,
			Interactive: false,
		}
		op, err := c.instServer.ExecInstance(name, req, nil)
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
