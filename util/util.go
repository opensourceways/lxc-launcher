package util

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func MergeConfigs(instConfig, newConfig map[string]string) map[string]string {
	for k, v := range newConfig {
		instConfig[k] = v
	}
	return instConfig
}

func MergeDeviceConfigs(instDevices, newDevices map[string]map[string]string) map[string]map[string]string {
	for k, v := range newDevices {
		if _, ok := instDevices[k]; ok {
			instDevices[k] = MergeConfigs(instDevices[k], v)
		} else {
			instDevices[k] = map[string]string{}
			instDevices[k] = MergeConfigs(instDevices[k], v)
		}
	}
	return instDevices
}

func CmdForLog(command string, args ...string) string {
	if strings.ContainsAny(command, " \t\n") {
		command = fmt.Sprintf("%q", command)
	}
	argsCopy := make([]string, len(args))
	copy(argsCopy, args)
	for i := range args {
		if strings.ContainsAny(args[i], " \t\n") {
			argsCopy[i] = fmt.Sprintf("%q", args[i])
		}
	}
	return command + " " + strings.Join(argsCopy, " ")
}

type cleanUp func()
type statusHandler func(w http.ResponseWriter, req *http.Request)

// ListenSignals Graceful start/stop server
func ListenSignals(cleanup cleanUp) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	go handleSignals(sigChan, cleanup)
}

// handleSignals handle process signal
func handleSignals(c chan os.Signal, cleanup cleanUp) {
	fmt.Println("Notice: System signal monitoring is enabled(watch: SIGINT,SIGTERM,SIGQUIT)")

	switch <-c {
	case syscall.SIGINT:
		fmt.Println("\nShutdown by Ctrl+C")
	case syscall.SIGTERM:
		fmt.Println("\nShutdown quickly")
	case syscall.SIGQUIT:
		fmt.Println("\nShutdown gracefully")
	}

	cleanup()

	os.Exit(0)
}

func ServerHealth(handler statusHandler, statusPort int64) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handler)
	statusServer := http.Server{
		Addr:           fmt.Sprintf("0.0.0.0:%d", statusPort),
		Handler:        mux,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	err := statusServer.ListenAndServe()
	if err != nil {
		fmt.Println(fmt.Sprintf("failed to setup status server %v", err))
	}
}

func GetImagePath(imageName string) string {
	return strings.Replace(strings.Replace(imageName, "/", "-", -1), ":", "-", -1)
}

func ReadContent(filePath string) (string, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func WriteContent(filePath, content string) error {
	return ioutil.WriteFile(filePath, []byte(content), 0644)
}
