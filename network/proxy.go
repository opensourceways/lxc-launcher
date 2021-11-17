package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/opensourceways/lxc-launcher/util"
	"go.uber.org/zap"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const MaxRetry = 3

type Proxy struct {
	instName     string
	BindAddress  string
	ProxyAddress string
	logger       *zap.Logger
	closed       bool
	CloseChannel chan bool
	socatBin     string
	portPairs    *map[int64]int64
}

func NewProxy(instName, bindAddress string, proxyAddress string, portPairs []string, logger *zap.Logger) (*Proxy, error) {
	socatBin, err := exec.LookPath("socat")
	if err != nil {
		err := errors.New("unable to find socat binary")
		logger.Error(err.Error())
		return nil, err
	}
	ports := map[int64]int64{}
	for _, p := range portPairs {
		values := strings.SplitN(p, ":", 2)
		if len(values) >= 2 {
			watchPort, err := strconv.ParseInt(values[0], 10, 64)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("failed to parse port pairs for proxying %s", values[0]))
			}
			proxyPort, err := strconv.ParseInt(values[1], 10, 64)
			if err != nil {
				return nil, errors.New(fmt.Sprintf("failed to parse port pairs for proxying %s", values[1]))
			}
			ports[watchPort] = proxyPort
		}
	}
	return &Proxy{
		instName:     instName,
		BindAddress:  bindAddress,
		ProxyAddress: proxyAddress,
		logger:       logger,
		closed:       false,
		CloseChannel: make(chan bool, 1),
		socatBin:     socatBin,
		portPairs:    &ports,
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

func (p *Proxy) Proxy(ctx context.Context, watchPort, proxyPort int64) error {
	args := []string{"-dd",
		fmt.Sprintf("TCP4-LISTEN:%d,bind=%s,reuseaddr,fork", watchPort, p.BindAddress),
		fmt.Sprintf("TCP4-CONNECT:%s:%d", p.ProxyAddress, proxyPort)}
	_, err := p.runCommandWithStdin(ctx, "", "", p.socatBin, args...)
	if err != nil {
		p.logger.Error(fmt.Sprintf("failed to perform socat bind operation for port %d", proxyPort))
		return err
	}
	return nil
}

func (p *Proxy) PerformProxy(ctx context.Context, watchPort, proxyPort int64) {
	retry := 1
	for {
		if p.closed {
			p.logger.Info(fmt.Sprintf("received cancel signal, quit network proxy..."))
			return
		}
		if retry <= MaxRetry {
			p.logger.Info(fmt.Sprintf(
				"loop perform network proxy (current: %d, max: %d) for instance %s and port %d", retry,
				MaxRetry, p.instName, proxyPort))
			err := p.Proxy(ctx, watchPort, proxyPort)
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
	for k, v := range *p.portPairs {
		go p.PerformProxy(ctx, k, v)
	}
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
