/*
 Copyright 2014 Canonical Ltd.
 Authors: James Henstridge <james.henstridge@canonical.com>

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

package accounts

/*
#cgo pkg-config: glib-2.0 libaccounts-glib libsignon-glib
#include <stdlib.h>
#include <glib.h>
#include "account-watcher.h"

AccountWatcher *watch_for_service_type(const char *service_type);
*/
import "C"
import (
	"errors"
	"sync"
	"unsafe"
)

type Watcher struct {
	C         <-chan AuthData
	AccountCh <-chan AccountData
	watcher   *C.AccountWatcher
}

type AccountData struct {
	AccountId   uint
	ServiceName string
}

type AuthData struct {
	AccountId   uint
	ServiceName string
	Error       error
	Enabled     bool

	ClientId     string
	ClientSecret string
	AccessToken  string
	TokenSecret  string
}

var (
	mainLoopOnce        sync.Once
	authChannels        = make(map[*C.AccountWatcher]chan<- AuthData)
	authChannelsLock    sync.Mutex
	accountChannels     = make(map[*C.AccountWatcher]chan<- AccountData)
	accountChannelsLock sync.Mutex
)

func StartGlibMainLoop() {
	mainLoopOnce.Do(func() {
		mainLoop := C.g_main_loop_new(nil, C.gboolean(1))
		go C.g_main_loop_run(mainLoop)
	})
}

// NewWatcher creates a new account watcher for the given service names
func NewWatcher(serviceType string) *Watcher {
	w := new(Watcher)
	cServiceType := C.CString(serviceType)
	defer C.free(unsafe.Pointer(cServiceType))
	w.watcher = C.watch_for_service_type(cServiceType)

	ch := make(chan AuthData)
	w.C = ch
	authChannelsLock.Lock()
	authChannels[w.watcher] = ch
	authChannelsLock.Unlock()
	// account stuff
	accountCh := make(chan AccountData)
	w.AccountCh = accountCh
	accountChannelsLock.Lock()
	accountChannels[w.watcher] = accountCh
	accountChannelsLock.Unlock()

	return w
}

// Refresh requests that the token for the given account be refreshed.
// The new access token will be delivered over the watcher's channel.
func (w *Watcher) Refresh(accountId uint) {
	C.account_watcher_refresh(w.watcher, C.uint(accountId))
}

//export authCallback
func authCallback(watcher unsafe.Pointer, accountId C.uint, serviceName *C.char, error *C.GError, enabled C.int, clientId, clientSecret, accessToken, tokenSecret *C.char, userData unsafe.Pointer) {
	// Ideally the first argument would be of type
	// *C.AccountWatcher, but that fails with Go 1.2.
	authChannelsLock.Lock()
	ch := authChannels[(*C.AccountWatcher)(watcher)]
	authChannelsLock.Unlock()
	if ch == nil {
		// Log the error
		return
	}

	var data AuthData
	data.AccountId = uint(accountId)
	data.ServiceName = C.GoString(serviceName)
	if error != nil {
		data.Error = errors.New(C.GoString((*C.char)(error.message)))
	}
	if enabled != 0 {
		data.Enabled = true
	}
	if clientId != nil {
		data.ClientId = C.GoString(clientId)
	}
	if clientSecret != nil {
		data.ClientSecret = C.GoString(clientSecret)
	}
	if accessToken != nil {
		data.AccessToken = C.GoString(accessToken)
	}
	if tokenSecret != nil {
		data.TokenSecret = C.GoString(tokenSecret)
	}
	ch <- data
}

//export accountCallback
func accountCallback(watcher unsafe.Pointer, accountId C.uint, serviceName *C.char, userData unsafe.Pointer) {
	// Ideally the first argument would be of type
	// *C.AccountWatcher, but that fails with Go 1.2.
	accountChannelsLock.Lock()
	ch := accountChannels[(*C.AccountWatcher)(watcher)]
	accountChannelsLock.Unlock()
	if ch == nil {
		// Log the error
		return
	}

	var data AccountData
	data.AccountId = uint(accountId)
	data.ServiceName = C.GoString(serviceName)
	ch <- data
}
