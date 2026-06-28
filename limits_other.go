//go:build !linux

package main

func maxOpenFiles() (int, bool, error) {
	return 0, false, nil
}

func currentOpenFiles() (int, bool, error) {
	return 0, false, nil
}
