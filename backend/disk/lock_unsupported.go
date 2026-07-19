//go:build !linux

package disk

import "fmt"

type FileLock struct{}

func NewFileLock(string) (*FileLock, error) {
	return nil, fmt.Errorf("host-local CNI storage is supported only on Linux")
}

func (*FileLock) Close() error  { return nil }
func (*FileLock) Lock() error   { return nil }
func (*FileLock) Unlock() error { return nil }
