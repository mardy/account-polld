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
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"log"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/gettext"
	"launchpad.net/account-polld/plugins"
	"launchpad.net/account-polld/plugins/gmail"
	"launchpad.net/account-polld/plugins/imap"
	"launchpad.net/account-polld/plugins/twitter"
	"launchpad.net/account-polld/pollbus"
	"launchpad.net/account-polld/qtcontact"
	"launchpad.net/go-dbus/v1"
)

type PostWatch struct {
	appId   plugins.ApplicationId
	batches []*plugins.PushMessageBatch
}

/* Use identifiers and API keys provided by the respective webapps which are the official
   end points for the notifications */
const (
	SERVICETYPE_WEBAPPS = "webapps"
	SERVICETYPE_IMAP    = "imap"

	SERVICENAME_GMAIL   = "com.ubuntu.developer.webapps.webapp-gmail_webapp-gmail"
	SERVICENAME_TWITTER = "com.ubuntu.developer.webapps.webapp-twitter_webapp-twitter"
)

const (
	POSTAL_SERVICE          = "com.ubuntu.Postal"
	POSTAL_INTERFACE        = "com.ubuntu.Postal"
	POSTAL_OBJECT_PATH_PART = "/com/ubuntu/Postal/"
)

var mainLoopOnce sync.Once

func init() {
	startMainLoop()
}

func startMainLoop() {
	mainLoopOnce.Do(func() {
		go qtcontact.MainLoopStart()
	})
}

func main() {
	// TODO NewAccount called here is just for playing purposes.
	postWatch := make(chan *PostWatch)

	// Initialize i18n
	gettext.SetLocale(gettext.LC_ALL, "")
	gettext.Textdomain("account-polld")
	gettext.BindTextdomain("account-polld", "/usr/share/locale")

	bus, err := dbus.Connect(dbus.SessionBus)
	if err != nil {
		log.Fatal("Cannot connect to bus", err)
	}

	pollBus := pollbus.New(bus)
	go postOffice(bus, postWatch)
	go monitorAccounts(postWatch, pollBus)

	if err := pollBus.Init(); err != nil {
		log.Fatal("Issue while setting up the poll bus:", err)
	}

	done := make(chan bool)
	<-done
}

func monitorAccounts(postWatch chan *PostWatch, pollBus *pollbus.PollBus) {
	// register account watchers
	webappWatcher := accounts.NewWatcher(SERVICETYPE_WEBAPPS)
	imapWatcher := accounts.NewWatcher(SERVICETYPE_IMAP)

	// map: account id -> account manager
	mgr := make(map[uint]*AccountManager)

	var wg sync.WaitGroup

	for {
		select {
		// Handle account creation, new account data and account deletion
		case data := <-webappWatcher.C:
			handleWatcherData(&wg, webappWatcher, postWatch, mgr, data, SERVICETYPE_WEBAPPS)
		case data := <-imapWatcher.C:
			handleWatcherData(&wg, imapWatcher, postWatch, mgr, data, SERVICETYPE_IMAP)
		// Respond to dbus poll requests
		case <-pollBus.PollChan:
			wg.Wait() // Finish all running Poll() calls before potentially polling the same accounts again
			for _, v := range mgr {
				if v.authData.Error != plugins.ErrTokenExpired { // Do not poll if the new token hasn't been loaded yet
					wg.Add(1)
					go func(accountManager *AccountManager) {
						defer wg.Done()
						accountManager.Poll(false)
					}(v)
				} else {
					log.Println("Skipping account with id", v.authData.AccountId, "as it is refreshing its token")
				}
			}
			wg.Wait()
			pollBus.SignalDone()
		}
	}
}

func handleWatcherData(wg *sync.WaitGroup, watcher *accounts.Watcher, postWatch chan *PostWatch, mgr map[uint]*AccountManager, data accounts.AuthData, serviceType string) {
	if account, ok := mgr[data.AccountId]; ok {
		if data.Enabled {
			log.Println("New account data for existing account with id", data.AccountId)
			account.penaltyCount = 0
			account.updateAuthData(data)
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Poll() needs to be called asynchronously as otherwise qtcontacs' GetAvatar() will
				// raise an error: "QSocketNotifier: Can only be used with threads started with QThread"
				account.Poll(false)
			}()
			// No wg.Wait() here as it would break GetAvatar() again.
			// Instead we have a wg.Wait() before the PollChan polling below.
		} else {
			account.Delete()
			delete(mgr, data.AccountId)
		}
	} else if data.Enabled {
		var plugin plugins.Plugin = nil
		if serviceType == SERVICETYPE_IMAP { // Allow any app to display imap notifications
			log.Println("Creating account with id", data.AccountId, "for", data.ServiceName)
			plugin = imap.New(data.ServiceName, data.AccountId)
		} else {
			switch data.ServiceName {
			case SERVICENAME_GMAIL:
				log.Println("Creating account with id", data.AccountId, "for", data.ServiceName)
				plugin = gmail.New(data.AccountId)
			case SERVICENAME_TWITTER:
				log.Println("Creating account with id", data.AccountId, "for", data.ServiceName)
				plugin = twitter.New(data.AccountId)
			default:
				log.Println("Unhandled account with id", data.AccountId, "for", data.ServiceName)
			}
		}
		if plugin != nil {
			mgr[data.AccountId] = NewAccountManager(watcher, postWatch, plugin)
			mgr[data.AccountId].updateAuthData(data)
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Poll() needs to be called asynchronously as otherwise qtcontacs' GetAvatar() will
				// raise an error: "QSocketNotifier: Can only be used with threads started with QThread"
				mgr[data.AccountId].Poll(true)
			}()
			// No wg.Wait() here as it would break GetAvatar() again.
			// Instead we have a wg.Wait() before the PollChan polling below.
		}
	}
}

func postOffice(bus *dbus.Connection, postWatch chan *PostWatch) {
	for post := range postWatch {
		obj := bus.Object(POSTAL_SERVICE, pushObjectPath(post.appId))

		for _, batch := range post.batches {

			notifs := batch.Messages
			overflowing := len(notifs) > batch.Limit

			for _, n := range notifs {
				// We're overflowing, so no popups.
				// See LP: #1527171
				if overflowing {
					n.Notification.Card.Popup = false
				}
			}

			if overflowing {
				n := batch.OverflowHandler(notifs)
				n.Notification.Card.Persist = false
				notifs = append(notifs, n)
			}

			for _, n := range notifs {
				var pushMessage string
				if out, err := json.Marshal(n); err == nil {
					pushMessage = string(out)
				} else {
					log.Printf("Cannot marshall %#v to json: %s", n, err)
					continue
				}
				if _, err := obj.Call(POSTAL_INTERFACE, "Post", post.appId, pushMessage); err != nil {
					log.Println("Cannot call the Post Office:", err)
					log.Println("Message missed posting:", pushMessage)
				}
			}
		}
	}
}

// pushObjectPath returns the object path of the ApplicationId
// for Push Notifications with the Quoted Package Name in the form of
// /com/ubuntu/PushNotifications/QUOTED_PKGNAME
//
// e.g.; if the APP_ID is com.ubuntu.music", the returned object path
// would be "/com/ubuntu/PushNotifications/com_2eubuntu_2eubuntu_2emusic
func pushObjectPath(id plugins.ApplicationId) dbus.ObjectPath {
	idParts := strings.Split(string(id), "_")
	if len(idParts) < 2 {
		panic(fmt.Sprintf("APP_ID '%s' is not valid", id))
	}

	pkg := POSTAL_OBJECT_PATH_PART
	for _, c := range idParts[0] {
		switch c {
		case '+', '.', '-', ':', '~', '_':
			pkg += fmt.Sprintf("_%x", string(c))
		default:
			pkg += string(c)
		}
	}
	return dbus.ObjectPath(pkg)
}
