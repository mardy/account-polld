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
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"log"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/plugins"
	"launchpad.net/account-polld/plugins/facebook"
	"launchpad.net/account-polld/plugins/gmail"
	"launchpad.net/account-polld/plugins/twitter"
	"launchpad.net/account-polld/qtcontact"
	"launchpad.net/go-dbus/v1"
)

type PostWatch struct {
	appId    plugins.ApplicationId
	messages []plugins.PushMessage
}

const (
	SERVICETYPE_POLL = "account-polld"

	SERVICENAME_GMAIL    = "google-gmail-poll"
	SERVICENAME_TWITTER  = "twitter-poll"
	SERVICENAME_FACEBOOK = "facebook-poll"
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

	if bus, err := dbus.Connect(dbus.SessionBus); err != nil {
		log.Fatal("Cannot connect to bus", err)
	} else {
		go postOffice(bus, postWatch)
	}

	go monitorAccounts(postWatch)

	done := make(chan bool)
	<-done
}

func monitorAccounts(postWatch chan *PostWatch) {
	watcher := accounts.NewWatcher(SERVICETYPE_POLL)
	mgr := make(map[uint]*AccountManager)
L:
	for data := range watcher.C {
		if account, ok := mgr[data.AccountId]; ok {
			if data.Enabled {
				log.Printf("New account data for %d - was %#v, now is %#v", data.AccountId, account.authData, data)
				account.updateAuthData(data)
			} else {
				account.Delete()
				delete(mgr, data.AccountId)
			}
		} else if data.Enabled {
			var plugin plugins.Plugin
			switch data.ServiceName {
			case SERVICENAME_GMAIL:
				log.Println("Creating account with id", data.AccountId, "for", data.ServiceName)
				plugin = gmail.New()
			case SERVICENAME_FACEBOOK:
				// This is just stubbed until the plugin exists.
				log.Println("Creating account with id", data.AccountId, "for", data.ServiceName)
				plugin = facebook.New()
			case SERVICENAME_TWITTER:
				// This is just stubbed until the plugin exists.
				log.Println("Creating account with id", data.AccountId, "for", data.ServiceName)
				plugin = twitter.New()
			default:
				log.Println("Unhandled account with id", data.AccountId, "for", data.ServiceName)
				continue L
			}
			mgr[data.AccountId] = NewAccountManager(watcher, postWatch, plugin)
			mgr[data.AccountId].updateAuthData(data)
			go mgr[data.AccountId].Loop()
		}
	}
}

func postOffice(bus *dbus.Connection, postWatch chan *PostWatch) {
	for post := range postWatch {
		for _, n := range post.messages {
			var pushMessage string
			if out, err := json.Marshal(n); err == nil {
				pushMessage = string(out)
			} else {
				log.Printf("Cannot marshall %#v to json: %s", n, err)
				continue
			}
			obj := bus.Object(POSTAL_SERVICE, pushObjectPath(post.appId))
			if _, err := obj.Call(POSTAL_INTERFACE, "Post", post.appId, pushMessage); err != nil {
				log.Println("Cannot call the Post Office:", err)
				log.Println("Message missed posting:", pushMessage)
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
