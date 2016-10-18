// Copyright 2016-2016 Platina Systems, Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license described in the
// LICENSE file.

// Package goesd provides `/usr/sbin/goesd` that starts a redis server and all
// of the configured daemons.
//
// If present, this sources `/etc/goesd` which set these variables.
//
//	REDISD		list of net devices that the server listens to
//			default: lo
//	MACHINED	machined arguments
package goesd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/platinasystems/go/command"
	"github.com/platinasystems/go/pidfile"
	"github.com/platinasystems/go/sockfile"
)

// If present, /etc/goesd is sourced before running redisd, machined, and
// the remaining damons.
const EtcGoesd = "/etc/goesd"

var ErrNotRoot = errors.New("you aren't root")

// Machines may use this Hook to run something before redisd, machined, etc.
var Hook = func() error { return nil }

type goesd struct{}

func New() goesd { return goesd{} }

func (goesd) String() string { return "/usr/sbin/goesd" }
func (goesd) Usage() string  { return "/usr/sbin/goesd" }

func (goesd) Daemon() int { return -1 }

func (goesd goesd) Main(args ...string) error {
	if os.Geteuid() != 0 {
		return ErrNotRoot
	}
	if len(args) > 0 {
		if args[0] == "stop" {
			return goesd.stop()
		}
		return fmt.Errorf("%v: unexpected", args)
	}
	err := Hook()
	if err != nil {
		return err
	}
	if _, err = os.Stat(EtcGoesd); err == nil {
		err = command.Main("source", EtcGoesd)
		if err != nil {
			return err
		}
	}
	args = strings.Fields(os.Getenv("REDISD"))
	if len(args) > 0 {
		err = command.Main(append([]string{"redisd"}, args...)...)
	} else {
		err = command.Main("redisd")
	}
	if err != nil {
		return err
	}
	args = strings.Fields(os.Getenv("MACHINED"))
	if len(args) > 0 {
		err = command.Main(append([]string{"machined"}, args...)...)
	} else {
		err = command.Main("machined")
	}
	if err != nil {
		return err
	}
	for daemon, lvl := range command.Daemon {
		if lvl < 0 {
			continue
		}
		err = command.Main(daemon)
		if err != nil {
			return err
		}
	}
	return nil
}

func (goesd) stop() error {
	thisprog, err := os.Readlink("/proc/self/exe")
	if err != nil {
		return err
	}
	thispid := os.Getpid()
	exes, err := filepath.Glob("/proc/*/exe")
	if err != nil {
		return err
	}
	var pids []int
	for _, exe := range exes {
		prog, err := os.Readlink(exe)
		if err != nil || prog != thisprog {
			continue
		}
		var pid int
		spid := strings.TrimPrefix(strings.TrimSuffix(exe, "/exe"),
			"/proc/")
		fmt.Sscan(spid, &pid)
		if pid != thispid {
			pids = append(pids, pid)
		}
	}
	for _, pid := range pids {
		_, err = os.Stat(fmt.Sprint("/proc/", pid, "/stat"))
		if err == nil {
			syscall.Kill(pid, syscall.SIGTERM)
		}
	}
	time.Sleep(2 * time.Second)
	for _, pid := range pids {
		_, err = os.Stat(fmt.Sprint("/proc/", pid, "/stat"))
		if err == nil {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	sockfile.RemoveAll()
	pidfile.RemoveAll()
	return nil
}
