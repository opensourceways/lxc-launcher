package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/opensourceways/lxc-launcher/util"
	"go.uber.org/zap"
	"os/exec"
	"syscall"
	"time"
)

const MaxRetry = 3

type Proxy struct {
	instName     string
	Address      string
	Port         int64
	WatchAddress string
	logger       *zap.Logger
	closed       bool
	CloseChannel chan bool
	socatBin     string
}

func NewProxy(instName, address string, port int64, watchAddress string, logger *zap.Logger) (*Proxy, error) {
	socatBin, err := exec.LookPath("socat")
	if err != nil {
		err := errors.New("unable to find socat binary")
		logger.Error(err.Error())
		return nil, err
	}
	return &Proxy{
		instName:     instName,
		Address:      address,
		Port:         port,
		WatchAddress: watchAddress,
		logger:       logger,
		closed:       false,
		CloseChannel: make(chan bool, 1),
		socatBin:     socatBin,
	}, nil
}

func (p *Proxy) runCommandWithStdin(ctx context.Context, cwd, stdin, command string, args ...string) (string, error) {
	cmdStr := util.CmdForLog(command, args...)
	p.logger.Info(fmt.Sprintf("running command cwd %s cmd %s", cwd, cmdStr))

	cmd := exec.CommandContext(ctx, command, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	outbuf := bytes.NewBuffer(nil)
	errbuf := bytes.NewBuffer(nil)
	cmd.Stdout = outbuf
	cmd.Stderr = errbuf
	cmd.Stdin = bytes.NewBufferString(stdin)

	err := cmd.Run()
	stdout := outbuf.String()
	stderr := errbuf.String()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("run(%s): %w: { stdout: %q, stderr: %q }", cmdStr, ctx.Err(), stdout, stderr)
	}
	if err != nil {
		return "", fmt.Errorf("run(%s): %w: { stdout: %q, stderr: %q }", cmdStr, err, stdout, stderr)
	}
	p.logger.Info(fmt.Sprintf("command result stdout %q, stderr %q", stdout, stderr))

	return stdout, nil
}

func (p *Proxy) Proxy(ctx context.Context) error {
	args := []string{"-dd",
		fmt.Sprintf("TCP4-LISTEN:%d,bind=%s,reuseaddr,fork", p.Port, p.Address),
		fmt.Sprintf("TCP4-CONNECT:%s", p.WatchAddress)}
	_, err := p.runCommandWithStdin(ctx, "", "", p.socatBin, args...)
	if err != nil {
		p.logger.Error("failed to perform socat bind operation")
		return err
	}
	return nil
}

func (p *Proxy) PerformProxy(ctx context.Context) {
	retry := 1
	for {
		if p.closed {
			p.logger.Info(fmt.Sprintf("received cancel signal, quit network proxy..."))
			return
		}
		if retry <= MaxRetry {
			p.logger.Info(fmt.Sprintf("loop perform network proxy (current: %d, max: %d) for instance %s", retry,
				MaxRetry, p.instName))
			err := p.Proxy(ctx)
			if err != nil {
				p.logger.Error(
					fmt.Sprintf("failed to perform network proxy for instance %s, %s", p.instName, err))
			}
		} else {
			break
		}
		retry += 1
	}
	p.logger.Error(fmt.Sprintf(
		"[Maximum reached] failed to perform network proxy for instance %s, app will exit, check log for detail",
		p.instName))
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
}

func (p *Proxy) StartLoop() {
	p.logger.Info(fmt.Sprintf("start to perform network proxy for instance %s", p.instName))
	//start watching with cancel context
	ctx, cancel := context.WithCancel(context.Background())
	go p.PerformProxy(ctx)
	for {
		select {
		case _, ok := <-p.CloseChannel:
			if !ok {
				cancel()
				time.Sleep(2 * time.Second)
				p.logger.Info(fmt.Sprintf(
					"network proxy for instance %s received close event, quiting..", p.instName))
				return
			}
		}
	}
}

func (p *Proxy) Close() {
	p.closed = true
	close(p.CloseChannel)
}
