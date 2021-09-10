package util

import (
	"fmt"
	"strings"
)

func MergeConfigs(instConfig, newConfig map[string]string) map[string]string {
	for k, v := range newConfig {
		instConfig[k] = v
	}
	return instConfig
}

func MergeDeviceConfigs(instDevices, newDevices map[string]map[string]string) map[string]map[string]string {
	for k, v := range newDevices {
		instDevices[k] = MergeConfigs(instDevices[k], v)
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
