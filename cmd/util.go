package cmd

import (
	"fmt"
	"strings"
)

const EnvPrefix = "LAUNCHER"

func GenerateEnvFlags(name string) string {
	return fmt.Sprintf("%s_%s", EnvPrefix, strings.Replace(strings.ToUpper(name), "-", "_", -1))
}
