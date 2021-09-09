package lxd

import (
	"errors"
	"fmt"
	"go.uber.org/zap"
	"strconv"
	"strings"

	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
)

const (
	STATUS_RUNNING = "Running"

	ACTION_STOP = "stop"
	ACTION_START = "start"
	SOURCE_TYPE_IMAGE = "image"
)

type ResourceLimit struct {
	Device string
	Name   string
	Value  string
}

type Client struct {
	instServer lxd.InstanceServer
	logger  *zap.Logger
	ResourceLimits []ResourceLimit
}

func NewClient(socket string, logger *zap.Logger) (*Client, error) {
	instServer, err := lxd.ConnectLXDUnix(socket, nil)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to connect lxd server via socket file, %s", err))
	}
	return &Client{
		instServer: instServer,
		logger: logger,
	}, nil
}

func (c *Client) ValidateResourceLimit(egressLimit, ingressLimit, rootSize, memoryResource, cpuResource string) error {
	//egress limitation
	if len(egressLimit) != 0 {
		if strings.HasSuffix(egressLimit, "Mbit") || strings.HasSuffix(
			egressLimit, "Gbit") || strings.HasSuffix(egressLimit, "Tbit") {
			c.ResourceLimits = append(c.ResourceLimits, ResourceLimit{
				Device: "eth0",
				Name: "limits.egress",
				Value: egressLimit,
			})
		} else {
			return errors.New(fmt.Sprintf("instance network egress limitation %s incorrect", egressLimit))
		}
	}
	//ingress limitation
	if len(ingressLimit) != 0 {
		if strings.HasSuffix(ingressLimit, "Mbit") || strings.HasSuffix(
			ingressLimit, "Gbit") || strings.HasSuffix(ingressLimit, "Tbit"){
			c.ResourceLimits = append(c.ResourceLimits, ResourceLimit{
				Device: "eth0",
				Name: "limits.ingress",
				Value: ingressLimit,
			})
		} else {
			return errors.New(fmt.Sprintf("instance network ingress limitation %s incorrect", ingressLimit))
		}
	}
	//root size
	if len(rootSize) != 0 {
		if strings.HasSuffix(rootSize, "MB") || strings.HasSuffix(
			rootSize, "GB") || strings.HasSuffix(rootSize, "TB") {
			c.ResourceLimits = append(c.ResourceLimits, ResourceLimit{
				Device: "root",
				Name: "size",
				Value: rootSize,
			})
		} else {
			return errors.New(fmt.Sprintf("instance storage size limitation %s incorrect", rootSize))
		}
	}
	//memory limitation
	if len(memoryResource) != 0 {
		if strings.HasSuffix(memoryResource, "MB") || strings.HasSuffix(
			memoryResource, "GB") || strings.HasSuffix(memoryResource, "TB") {
			c.ResourceLimits = append(c.ResourceLimits, ResourceLimit{
				Device: "memory",
				Name: "limits.memory",
				Value: memoryResource,
			})
		} else {
			return errors.New(fmt.Sprintf("instance memory limitation %s incorrect", memoryResource))
		}
	}
	//cpu limitation
	if len(cpuResource) != 0 {
		if strings.HasSuffix(cpuResource, "%") {
			c.ResourceLimits = append(c.ResourceLimits, ResourceLimit{
				Device: "cpu",
				Name: "limits.cpu",
				Value: "1",
			})
			c.ResourceLimits = append(c.ResourceLimits, ResourceLimit{
				Device: "cpu",
				Name: "limits.cpu.allowance",
				Value: cpuResource,
			})
		} else {
			core, err := strconv.Atoi(cpuResource)
			if err != nil {
				return err
			}
			if core < 1 {
				return errors.New("cpu core must be equal or greater than 1")
			}
			c.ResourceLimits = append(c.ResourceLimits, ResourceLimit{
				Device: "cpu",
				Name: "limits.cpu",
				Value: cpuResource,
			})
		}
	}
	rlimits := "Instance resource limit: "
	for _, l := range c.ResourceLimits {
		rlimits += fmt.Sprintf("device:%s,name:%s,value:%s;", l.Device, l.Name, l.Value)
	}
	c.logger.Info(rlimits)
	return nil
}

func (c *Client)CheckPoolExists(name string) (bool, error) {
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

func (c *Client)LaunchInstance(name string) error {
	instance, etag, err := c.instServer.GetInstance(name)
	if err != nil {
		return err
	}
	if instance.StatusCode == api.Running {
		c.logger.Info(fmt.Sprintf("instance %s already running.", name))
		return nil
	}
	if instance.StatusCode == api.Error || instance.StatusCode.IsFinal() {
		return errors.New(fmt.Sprintf("instance %s in %s state", name, instance.Status))
	}
	if instance.StatusCode == api.Stopped {
		req := api.InstanceStatePut{
			Action: ACTION_START,
			Timeout: -1,
			Force: true,
			Stateful: false,
		}
		op, err := c.instServer.UpdateInstanceState(name, req, etag)
		if err != nil {
			return err
		}
		return op.Wait()
	}
	return nil
}

func (c *Client)DeleteInstance(name string) error {
	instance,etag, err := c.instServer.GetInstance(name)
	if err != nil {
		return err
	}
	if instance.Status == STATUS_RUNNING {
		req := api.InstanceStatePut{
			Action: ACTION_STOP,
			Timeout: -1,
			Force: true,
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
	op, err := c.instServer.DeleteInstance(name)
	if err != nil {
		return err
	}
	return op.Wait()
}

func (c *Client) CreateInstance(imageAlias string, instanceName string) error {
	req := api.InstancesPost{
		Name: instanceName,
		Source: api.InstanceSource{
			Type: SOURCE_TYPE_IMAGE ,
			Alias: imageAlias,
		},
	}
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

func (c *Client)CheckInstanceExists(name string, containerOnly bool) (bool, error) {
	instanceType := api.InstanceTypeAny
	if containerOnly {
		instanceType = api.InstanceTypeContainer
	}
	names, err := c.instServer.GetInstanceNames(instanceType)
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


