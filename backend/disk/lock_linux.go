// Copyright 2015 CNI authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux

package disk

import (
	"os"
	"syscall"
)

type FileLock struct {
	file *os.File
}

func NewFileLock(path string) (*FileLock, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &FileLock{file: file}, nil
}

func (lock *FileLock) Close() error {
	return lock.file.Close()
}

func (lock *FileLock) Lock() error {
	return syscall.Flock(int(lock.file.Fd()), syscall.LOCK_EX)
}

func (lock *FileLock) Unlock() error {
	return syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
}
