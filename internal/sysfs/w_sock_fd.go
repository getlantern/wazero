//go:build (linux || darwin || windows) && !tinygo

// Copyright 2024 The WATER Authors. All rights reserved.
// Use of this source code is governed by Apache 2 license
// that can be found in the LICENSE file.

package sysfs

// Fd implements the same method as documented on fsapi.File
func (f *tcpListenerFile) Fd() uintptr {
	return f.cachedFd
}

// Fd implements the same method as documented on fsapi.File
func (f *tcpConnFile) Fd() uintptr {
	return f.cachedFd
}
