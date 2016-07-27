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
	"log"
	"time"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/plugins"
	"launchpad.net/ubuntu-push/click"
)

type AccountService struct {
	watcher          *accounts.Watcher
	authData         accounts.AuthData
	plugin           plugins.Plugin
	interval         time.Duration
	postWatch        chan *PostWatch
	authChan         chan accounts.AuthData
	doneChan         chan error
	penaltyCount     int
	authFailureCount int
}

var (
	pollTimeout           = time.Duration(30 * time.Second)
	bootstrapPollTimeout  = time.Duration(4 * time.Minute)
	maxCounter            = 4
	authTriesUntilPenalty = 3
	authFailurePenalty    = 10
)

var (
	authError              = errors.New("Skipped account")
	clickNotInstalledError = errors.New("Click not installed")
)

func NewAccountService(watcher *accounts.Watcher, postWatch chan *PostWatch, plugin plugins.Plugin) *AccountService {
	return &AccountService{
		watcher:   watcher,
		plugin:    plugin,
		postWatch: postWatch,
		authChan:  make(chan accounts.AuthData, 1),
		doneChan:  make(chan error, 1),
	}
}

func (a *AccountService) Delete() {
	close(a.authChan)
	close(a.doneChan)
}

// Poll() always needs to be called asynchronously as otherwise qtcontacs' GetAvatar()
// will raise an error: "QSocketNotifier: Can only be used with threads started with QThread"
func (a *AccountService) Poll(bootstrap bool) {
	gotNewAuthData := false
	if a.authData, gotNewAuthData = <-a.authChan; !gotNewAuthData {
		log.Println("Account", a.authData.AccountId, "no longer enabled")
		return
	}

	if a.penaltyCount > 0 {
		log.Printf("Leaving poll for account %d as penalty count is %d", a.authData.AccountId, a.penaltyCount)
		a.penaltyCount--
		return
	} else if !gotNewAuthData && a.authData.Error != nil {
		// Retry to poll the account with a previous auth failure as that results in reauthentication in case of token expiry and in ignoring temporary network issues
		log.Println("Retrying to poll account with previous auth failure and id", a.authData.AccountId, "(results in reauthentication in case of token expiry and in ignoring temporary network issues)")
		a.authData.Error = nil
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
			a.authFailureCount = 0
			a.penaltyCount = 0
		} else {
			if err != clickNotInstalledError && err != authError { // Do not log the error twice
				log.Println("Poll for account", a.authData.AccountId, "has failed:", err)
			}
			if err == authError || err == plugins.ErrTokenExpired {
				// Increase the authFailureCount counter, except for when we did a poll which
				// raised a token expiry error when we did not get any new auth data this time.
				if err != plugins.ErrTokenExpired || gotNewAuthData {
					log.Println("Increasing the auth failure counter for account", a.authData.AccountId)
					a.authFailureCount++
				} else {
					log.Println("Not increasing the auth failure counter for account", a.authData.AccountId, "as we do not have new auth data")
				}
				if a.authFailureCount >= authTriesUntilPenalty {
					a.penaltyCount = authFailurePenalty
					a.authFailureCount = 0
					log.Println(authTriesUntilPenalty, "auth failures in a row for account", a.authData.AccountId, "-> skipping it for the next", a.penaltyCount, "poll cycles")
				} else if err == plugins.ErrTokenExpired && !gotNewAuthData {
					// If the error indicates that the authentication token has expired, request reauthentication
					// and mark the data as disabled.
					// Do not refresh immediately when we just got new (faulty) auth data as immediately trying
					// again is probably not going to help. Instead, we wait for the next poll cycle.
					a.watcher.Refresh(a.authData.AccountId, a.authData.ServiceName)
					a.authData.Enabled = false
					a.authData.Error = err
				}
			} else if a.penaltyCount < maxCounter {
				a.authFailureCount = 0
				a.penaltyCount++
			}
		}
	}
	log.Printf("Ending poll for account %d", a.authData.AccountId)
}

func (a *AccountService) poll() {
	log.Println("Polling account", a.authData.AccountId)
	if !isClickInstalled(a.plugin.ApplicationId()) {
		log.Println(
			"Skipping account", a.authData.AccountId, "as target click",
			a.plugin.ApplicationId(), "is not installed")
		a.doneChan <- clickNotInstalledError
		return
	}

	if a.authData.Error != nil {
		log.Println("Account", a.authData.AccountId, "failed to authenticate:", a.authData.Error)
		a.doneChan <- authError
		return
	}

	if bs, err := a.plugin.Poll(&a.authData); err != nil {
		log.Print("Error while polling ", a.authData.AccountId, ": ", err)
		a.doneChan <- err
	} else {
		for _, b := range bs {
			log.Println("Account", a.authData.AccountId, "has", len(b.Messages), b.Tag, "updates to report")
		}
		a.postWatch <- &PostWatch{batches: bs, appId: a.plugin.ApplicationId()}
		a.doneChan <- nil
	}
}

func (a *AccountService) updateAuthData(authData accounts.AuthData) {
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
