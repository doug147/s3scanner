//go:build linux

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func maxOpenFiles() (int, bool, error) {
	var rlimit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rlimit); err != nil {
		return 0, true, err
	}
	maxInt := int(^uint(0) >> 1)
	if rlimit.Cur > uint64(maxInt) {
		return maxInt, true, nil
	}
	return int(rlimit.Cur), true, nil
}

func currentOpenFiles() (int, bool, error) {
	data, err := os.ReadFile("/proc/sys/fs/file-nr")
	if err != nil {
		return 0, true, err
	}
	parts := strings.Fields(string(data))
	if len(parts) < 1 {
		return 0, true, fmt.Errorf("unexpected content in /proc/sys/fs/file-nr")
	}
	openFiles, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, true, err
	}
	return openFiles, true, nil
}
