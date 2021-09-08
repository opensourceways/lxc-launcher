package lxd

import (
	"errors"
	"fmt"
	"go.uber.org/zap"

	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
)

type Client struct {
	instServer lxd.InstanceServer
	logger  *zap.Logger
}

func NewClient(socket string, logger *zap.Logger) (*Client, error) {
	instServer, err := lxd.ConnectLXD(socket, nil)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to connect lxd server via socket file"))
	}
	return &Client{
		instServer: instServer,
		logger: logger,
	}, nil
}

func (c *Client)DeleteInstance(name string) error {
	op, err := c.instServer.DeleteInstance(name)
	if err != nil {
		return err
	}
	err = op.Wait()
	if err != nil {
		return err
	}
	return nil
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


