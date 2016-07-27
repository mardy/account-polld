/*
 Copyright 2014 Canonical Ltd.

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

AccountWatcher *watch();
*/
import "C"
import (
	"errors"
	"sync"
	"unsafe"
)

type Watcher struct {
	C       <-chan AuthData
	watcher *C.AccountWatcher
}

type AuthData struct {
	AccountId   uint
	ServiceName string
	ServiceType string
	Error       error
	Enabled     bool

	ClientId     string
	ClientSecret string
	AccessToken  string
	TokenSecret  string
}

var (
	authChannels     = make(map[*C.AccountWatcher]chan<- AuthData)
	authChannelsLock sync.Mutex
)

// NewWatcher creates a new account watcher
func NewWatcher() *Watcher {
	w := new(Watcher)
	w.watcher = C.watch()

	ch := make(chan AuthData)
	w.C = ch
	authChannelsLock.Lock()
	authChannels[w.watcher] = ch
	authChannelsLock.Unlock()

	return w
}

func (w *Watcher) AddService(serviceId string) {
	C.account_watcher_add_service(w.watcher, C.CString(serviceId))
}

// Walk through the enabled accounts, and get auth tokens for each of them.
// The new access token will be delivered over the watcher's channel.
func (w *Watcher) Run() {
	C.account_watcher_run(w.watcher)
}

// Refresh requests that the token for the given account be refreshed.
// The new access token will be delivered over the watcher's channel.
func (w *Watcher) Refresh(accountId uint, serviceName string) {
	C.account_watcher_refresh(w.watcher, C.uint(accountId), C.CString(serviceName))
}

//export authCallback
func authCallback(watcher unsafe.Pointer, accountId C.uint, serviceType *C.char, serviceName *C.char, error *C.GError, enabled C.int, clientId, clientSecret, accessToken, tokenSecret *C.char, userData unsafe.Pointer) {
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
	data.ServiceType = C.GoString(serviceType)
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
