package common

import (
	"fmt"
	"lxc-launcher/log"
	"strings"
	"time"
)

const (
	DATE_T_FORMAT = "2006-01-02T15:04:05"
	DATE_FORMAT   = "2006-01-02 15:04:05"
)

func TimeStrToInt(timeStr string) int64 {
	if timeStr == "" {
		return 0
	}
	if timeStr != "" && len(timeStr) > 19 {
		timeStr = timeStr[:19]
	}
	layout := DATE_FORMAT
	if strings.Contains(timeStr, "T") {
		layout = DATE_T_FORMAT
	}
	loc, _ := time.LoadLocation("Local")
	theTime, err := time.ParseInLocation(layout, timeStr, loc)
	if err == nil {
		unixTime := theTime.Unix()
		return unixTime
	} else {
		log.Logger.Error(fmt.Sprintf("err: %v", err))
	}
	return 0
}

func GetCurTime() string {
	return time.Now().Format(DATE_FORMAT)
}
