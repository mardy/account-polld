/*
 Copyright 2014 Canonical Ltd.
 Copyright 2015 Simon Fels <morphis@gravedo.de>

 This program is free software: you can redistribute it and/or modify it
 under the terms of the GNU General Public License version 3, as published
 by the Free Software Foundation.

 This program is distributed in the hope that it will be useful, but
 WITHOUT ANY WARRANTY; without even the implied warranties of
 MERCHANTABILITY, SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR
 PURPOSE.  See the GNU General Public License for more details.

 You should have received a copy of the GNU General Public License along
 with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"time"
	"fmt"

	"launchpad.net/go-xdg/v0"

	"launchpad.net/account-polld/plugins"
	"launchpad.net/ubuntu-push/logger"
	"launchpad.net/ubuntu-push/launch_helper/cual"
	"launchpad.net/ubuntu-push/click"
)

var InputBufferSize = 10

var ErrCantFindHelper   = errors.New("can't find helper")
var ErrCantFindLauncher = errors.New("can't find launcher for helper")

type HelperPool interface {
	Run(appId, exec string)
	Start() chan *HelperResult
	Stop()
}

type HelperLauncher interface {
	InstallObserver(done func(string)) error
	RemoveObserver() error
	Launch(appId string, exec string, f1 string, f2 string) (string, error)
	Stop(appId string, instanceId string) error
}

type HelperResult struct {
	Success bool
	AppId string
	Messages []*plugins.PushMessage
}

type HelperInput struct {
	AppId string
	Exec string
}

type HelperArgs struct {
	Input      *HelperInput
	AppId      string
	FileOut    string
	Timer      *time.Timer
	ForcedStop bool
}

type helperPoolState struct {
	log logger.Logger
	chOut chan *HelperResult
	chIn chan *HelperInput
	chDone chan string
	chStopped chan struct{}
	maxRuntime time.Duration
	maxNum int
	launcher HelperLauncher
	lock sync.Mutex
	hmap map[string]*HelperArgs
}

func NewHelperPool() HelperPool {
	return &helperPoolState{
		maxRuntime: 5 * time.Second,
		maxNum: 5,
		hmap: make(map[string]*HelperArgs),
	}
}

func (pool *helperPoolState) Start() chan *HelperResult {
	pool.log = logger.NewSimpleLogger(os.Stderr, "info")
	pool.chOut = make(chan *HelperResult)
	pool.chIn = make(chan *HelperInput, InputBufferSize)
	pool.chDone = make(chan string)
	pool.chStopped = make(chan struct{})

	pool.launcher = cual.New(pool.log)
	err := pool.launcher.InstallObserver(func(iid string) {
		pool.OneDone(iid)
	})
	if err != nil {
		panic(fmt.Errorf("failed to install helper observer: %v", err))
	}

	go pool.loop()

	return pool.chOut
}

func (pool *helperPoolState) loop() {
	running := make(map[string]bool)
	var backlog []*HelperInput

	for {
		select {
		case in, ok := <-pool.chIn:
			if !ok {
				close(pool.chStopped)
				return
			}
			if len(running) >= pool.maxNum || running[in.AppId] {
				backlog = append(backlog, in)
				pool.log.Debugf("current helper input backlog has grown to %d entries.", len(backlog))
			} else {
				if pool.tryOne(in) {
					running[in.AppId] = true
				}
			}
		case appId := <-pool.chDone:
			delete(running, appId)
			if len(backlog) == 0 {
				continue
			}
			backlogSz := 0
			done := false
			for i, in := range backlog {
				if in != nil {
					if !done && !running[in.AppId] {
						backlog[i] = nil
						if pool.tryOne(in) {
							running[in.AppId] = true
							done = true
						}
					} else {
						backlogSz++
					}
				}
			}
			backlog = pool.shrinkBacklog(backlog, backlogSz)
			pool.log.Debugf("current helper input backlog has shrunk to %d entries.", backlogSz)
		}
	}
}

func (pool *helperPoolState) shrinkBacklog(backlog []*HelperInput, backlogSz int) []*HelperInput {
	if backlogSz == 0 {
		return nil
	}
	if cap(backlog) < 2*backlogSz {
		return backlog
	}
	pool.log.Debugf("copying backlog to avoid wasting too much space (%d/%d used)", backlogSz, cap(backlog))
	clean := make([]*HelperInput, 0, backlogSz)
	for _, bentry := range backlog {
		if bentry != nil {
			clean = append(clean, bentry)
		}
	}
	return clean
}

func (pool *helperPoolState) Stop() {
	close(pool.chIn)
	pool.launcher.RemoveObserver()

	// make Stop sync for tests
	<-pool.chStopped
}

func (pool *helperPoolState) Run(appId, exec string) {
	input := &HelperInput{ AppId: appId, Exec: exec }
	pool.chIn <- input
}

func (pool *helperPoolState) tryOne(input *HelperInput) bool {
	if pool.handleOne(input) != nil {
		pool.failOne(input)
		return false
	}
	return true
}

func (pool *helperPoolState) failOne(input *HelperInput) {
	pool.log.Errorf("unable to get helper output")
	pool.chOut <- &HelperResult{ Success: false, AppId: input.AppId }
}

func (pool *helperPoolState) cleanupTempFile(f1 string) {
	if f1 != "" {
		os.Remove(f1)
	}
}

func (pool *helperPoolState) handleOne(input *HelperInput) error {
	if input.AppId == "" && input.Exec == "" {
		pool.log.Errorf("can't locate helper for app")
		return ErrCantFindHelper
	}

	pool.log.Debugf("using helper %s (exec: %s)", input.AppId, input.Exec)

	fout, err := pool.createOutputTempFile(input)
	defer func() {
		if err != nil {
			pool.cleanupTempFile(fout)
		}
	}()
	if err != nil {
		pool.log.Errorf("unable to create output tempfile: %v", err)
		return err
	}

	args := HelperArgs{
		AppId: input.AppId,
		Input: input,
		FileOut: fout,
	}

	pool.lock.Lock()
	defer pool.lock.Unlock()

	iid, err := pool.launcher.Launch(input.AppId, input.Exec, fout, "")
	if err != nil {
		pool.log.Errorf("unable to launch helper %s: %v", input.AppId, err)
		return err
	}

	helperAppId := input.AppId

	uid := iid
	args.Timer = time.AfterFunc(pool.maxRuntime, func() {
		pool.peekId(uid, func(a *HelperArgs) {
			a.ForcedStop = true
			err := pool.launcher.Stop(helperAppId, iid)
			if err != nil {
				pool.log.Errorf("unable to forcefully stop helper %s: %v", helperAppId, err)
			}
		})
	})
	pool.hmap[uid] = &args

	return nil
}

func (pool *helperPoolState) peekId(uid string, cb func(*HelperArgs)) *HelperArgs {
	pool.lock.Lock()
	defer pool.lock.Unlock()
	args, ok := pool.hmap[uid]
	if ok {
		cb(args)
		return args
	}
	return nil
}

func (pool *helperPoolState) OneDone(uid string) {
	args := pool.peekId(uid, func(a *HelperArgs) {
		a.Timer.Stop()
		// dealt with, remove it
		delete(pool.hmap, uid)
	})
	if args == nil {
		// nothing to do
		return
	}
	pool.chDone <- args.AppId
	defer func() {
		pool.cleanupTempFile(args.FileOut)
	}()
	if args.ForcedStop {
		pool.failOne(args.Input)
		return
	}
	payload, err := ioutil.ReadFile(args.FileOut)
	if err != nil {
		pool.log.Errorf("unable to read output from %v helper: %v", args.AppId, err)
	} else {
		pool.log.Infof("%v helper output: %s", args.AppId, payload)
		res := &HelperResult{AppId: args.AppId}
		err = json.Unmarshal(payload, &res.Messages)
		if err != nil {
			pool.log.Errorf("failed to parse HelperOutput from %v helper output: %v", args.AppId, err)
		} else {
			pool.chOut <- res
		}
	}
	if err != nil {
		pool.failOne(args.Input)
	}
}

func (pool *helperPoolState) createOutputTempFile(input *HelperInput) (string, error) {
	app, err := click.ParseAppId(input.AppId)
	if err != nil {
		return "", errors.New("Could not determine package for app")
	}
	return getTempFilename(app.Package)
}

// helper helpers:

var xdgCacheHome = xdg.Cache.Home

func _getTempDir(pkgName string) (string, error) {
	tmpDir := path.Join(xdgCacheHome(), pkgName)
	err := os.MkdirAll(tmpDir, 0700)
	return tmpDir, err
}

// override GetTempDir for testing without writing to ~/.cache/<pkgName>
var GetTempDir func(pkgName string) (string, error) = _getTempDir

func _getTempFilename(pkgName string) (string, error) {
	tmpDir, err := GetTempDir(pkgName)
	if err != nil {
		return "", err
	}
	file, err := ioutil.TempFile(tmpDir, "push-helper")
	if err != nil {
		return "", err
	}
	defer file.Close()
	return file.Name(), nil
}

var getTempFilename = _getTempFilename
