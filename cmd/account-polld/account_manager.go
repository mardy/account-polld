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

package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/plugins"
	"launchpad.net/ubuntu-push/click"
	"launchpad.net/ubuntu-push/click/cblacklist"
)

type AccountManager struct {
	watcher      *accounts.Watcher
	authData     accounts.AuthData
	plugin       plugins.Plugin
	interval     time.Duration
	postWatch    chan *PostWatch
	authChan     chan accounts.AuthData
	doneChan     chan error
	penaltyCount int
}

var (
	pollTimeout          = time.Duration(30 * time.Second)
	bootstrapPollTimeout = time.Duration(4 * time.Minute)
	maxCounter           = 4
)

var (
	authError              = errors.New("Skipped account")
	clickNotInstalledError = errors.New("Click not installed")
)

var isBlacklisted = cblacklist.IsBlacklisted

func NewAccountManager(watcher *accounts.Watcher, postWatch chan *PostWatch, plugin plugins.Plugin) *AccountManager {
	return &AccountManager{
		watcher:   watcher,
		plugin:    plugin,
		postWatch: postWatch,
		authChan:  make(chan accounts.AuthData, 1),
		doneChan:  make(chan error, 1),
	}
}

func (a *AccountManager) Delete() {
	close(a.authChan)
	close(a.doneChan)
}

func (a *AccountManager) Poll(bootstrap bool) {
	if !a.authData.Enabled {
		var ok bool
		if a.authData, ok = <-a.authChan; !ok {
			log.Println("Account", a.authData.AccountId, "no longer enabled")
			return
		}
	}

	if id, ok := click.ParseAppId(string(a.plugin.ApplicationId())); (ok == nil) && isBlacklisted(id) {
		log.Printf("Account %d is blacklisted, not polling", a.authData.AccountId)
		return
	}

	if a.penaltyCount > 0 {
		log.Printf("Leaving poll for account %d as penalty count is %d", a.authData.AccountId, a.penaltyCount)
		a.penaltyCount--
		return
	}
	timeout := pollTimeout
	if bootstrap {
		timeout = bootstrapPollTimeout
	}

	log.Printf("Starting poll for account %d", a.authData.AccountId)
	go a.poll()

	select {
	case <-time.After(timeout):
		log.Println("Poll for account", a.authData.AccountId, "has timed out out after", timeout)
		a.penaltyCount++
	case err := <-a.doneChan:
		if err == nil {
			log.Println("Poll for account", a.authData.AccountId, "was successful")
			a.penaltyCount = 0
		} else {
			if err != clickNotInstalledError && err != authError { // Do not log the error twice
				log.Println("Poll for account", a.authData.AccountId, "has failed:", err)
			}
			if a.penaltyCount < maxCounter {
				a.penaltyCount++
			}
		}
	}
	log.Printf("Ending poll for account %d", a.authData.AccountId)
}

func (a *AccountManager) poll() {
	log.Println("Polling account", a.authData.AccountId)
	if !isClickInstalled(a.plugin.ApplicationId()) {
		log.Println(
			"Skipping account", a.authData.AccountId, "as target click",
			a.plugin.ApplicationId(), "is not installed")
		a.doneChan <- clickNotInstalledError
		return
	}

	if a.authData.Error != nil {
		log.Println("Account", a.authData.AccountId, ", ", a.authData.ClientId, ", ", a.authData.ClientSecret)
		log.Println("Account", a.authData.AccountId, "failed to authenticate:", a.authData.Error)
		a.doneChan <- authError
		return
	}

	if bs, err := a.plugin.Poll(&a.authData); err != nil {
		log.Print("Error while polling ", a.authData.AccountId, ": ", err)

		// If the error indicates that the authentication
		// token has expired, request reauthentication and
		// mark data as disabled.
		if err == plugins.ErrTokenExpired {
			a.watcher.Refresh(a.authData.AccountId)
			a.authData.Enabled = false
			a.authData.Error = err
		}
		a.doneChan <- err
	} else {
		for _, b := range bs {
			log.Println("Account", a.authData.AccountId, "has", len(b.Messages), b.Tag, "updates to report")
		}
		log.Println(fmt.Sprintf("PostWatch: %#v", PostWatch{batches: bs, appId: a.plugin.ApplicationId()}))
		a.postWatch <- &PostWatch{batches: bs, appId: a.plugin.ApplicationId()}
		a.doneChan <- nil
	}
}

func (a *AccountManager) updateAuthData(authData accounts.AuthData) {
	a.authChan <- authData
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
