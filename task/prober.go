package task

import (
	"fmt"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"lxc-launcher/lxd"
	"time"
)

var (
	MaxFailure = 2
)

type Prober struct {
	instName     string
	client       *lxd.Client
	interval     int32
	alive        *atomic.Int32
	logger       *zap.Logger
	closeChannel chan bool
}

func NewProber(name string, client *lxd.Client, interval int32, logger *zap.Logger) (*Prober, error) {
	return &Prober{
		instName:     name,
		client:       client,
		interval:     interval,
		alive:        atomic.NewInt32(0),
		logger:       logger,
		closeChannel: make(chan bool, 1),
	}, nil
}

func (p *Prober) Alive() bool {
	return !(int(p.alive.Load()) > MaxFailure)
}

func (p *Prober) probeInstance() {
	status, err := p.client.GetInstanceStatus(p.instName)
	if err != nil {
		p.logger.Error(fmt.Sprintf("failed to get instance status %s %v", p.instName, err))
	}
	if status == lxd.STATUS_RUNNING {
		p.setAlive(true)
	} else {
		p.logger.Warn(fmt.Sprintf("instance %s incorrect status %s", p.instName, status))
		p.setAlive(false)
	}
}

func (p *Prober) setAlive(alive bool) {
	if alive {
		p.alive.Store(0)
	} else {
		p.alive.Add(1)
	}
}

func (p *Prober) Close() {
	close(p.closeChannel)
}

func (p *Prober) StartLoop() {
	p.logger.Info(fmt.Sprintf("start to perform instance probe %s", p.instName))
	ticker := time.NewTicker(time.Duration(p.interval) * time.Second)
	for {
		select {
		case <-ticker.C:
			p.probeInstance()
		case _, ok := <-p.closeChannel:
			if !ok {
				p.logger.Info(fmt.Sprintf(
					"instance probe for instance %s received close event, quiting..", p.instName))
				return
			}
		}
	}
}
