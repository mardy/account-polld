/*
 Copyright 2014 Canonical Ltd.
 Authors: Sergio Schvezov <sergio.schvezov@canonical.com>

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
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/plugins"
	"launchpad.net/ubuntu-push/click"
)

type AccountManager struct {
	authData  accounts.AuthData
	authMutex *sync.Mutex
	plugin    plugins.Plugin
	interval  time.Duration
	postWatch chan *PostWatch
	terminate chan bool
}

var (
	pollInterval = time.Duration(5 * time.Minute)
	maxInterval  = time.Duration(20 * time.Minute)
)

func init() {
	if intervalEnv := os.Getenv("ACCOUNT_POLLD_POLL_INTERVAL_MINUTES"); intervalEnv != "" {
		if interval, err := strconv.ParseInt(intervalEnv, 0, 0); err == nil {
			pollInterval = time.Duration(interval) * time.Minute
		}
	}
}

func NewAccountManager(authData accounts.AuthData, postWatch chan *PostWatch, plugin plugins.Plugin) *AccountManager {
	return &AccountManager{
		plugin:    plugin,
		authData:  authData,
		authMutex: &sync.Mutex{},
		postWatch: postWatch,
		interval:  pollInterval,
		terminate: make(chan bool),
	}
}

func (a *AccountManager) Delete() {
	a.terminate <- true
}

func (a *AccountManager) Loop() {
	defer close(a.terminate)
L:
	for {
		log.Println("Polling set to", a.interval, "for", a.authData.AccountId)
		select {
		case <-time.After(a.interval):
			a.poll()
		case <-a.terminate:
			break L
		}
	}
	log.Printf("Ending poll loop for account %d", a.authData.AccountId)
}

func (a *AccountManager) poll() {
	a.authMutex.Lock()
	defer a.authMutex.Unlock()

	if !isClickInstalled(a.plugin.ApplicationId()) {
		log.Println(
			"Skipping account", a.authData.AccountId, "as target click",
			a.plugin.ApplicationId(), "is not installed")
		return
	}

	if !a.authData.Enabled {
		log.Println("Account", a.authData.AccountId, "no longer enabled")
		return
	}

	if n, err := a.plugin.Poll(&a.authData); err != nil {
		log.Print("Error while polling ", a.authData.AccountId, ": ", err)
		// penalizing the next poll
		if a.interval.Minutes() < maxInterval.Minutes() {
			a.interval += pollInterval
		}
	} else if len(n) > 0 {
		// on success we reset the timeout to the default interval
		a.interval = pollInterval
		a.postWatch <- &PostWatch{messages: n, appId: a.plugin.ApplicationId()}
	}
}

func (a *AccountManager) updateAuthData(authData accounts.AuthData) {
	a.authMutex.Lock()
	defer a.authMutex.Unlock()
	a.authData = authData
}

func isClickInstalled(appId plugins.ApplicationId) bool {
	user, err := click.User()
	if err != nil {
		log.Println("User instance for click cannot be created to determine if click application", appId, "was installed")
		return false
	}

	app, err := click.ParseAppId(string(appId))
	if err != nil {
		log.Println("Could not parse APP_ID for", appId)
		return false
	}

	return user.Installed(app, false)
}
