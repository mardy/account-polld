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
	"time"

	"log"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/gettext"
	"launchpad.net/account-polld/plugins"
	"launchpad.net/account-polld/plugins/facebook"
	"launchpad.net/account-polld/plugins/gmail"
	"launchpad.net/account-polld/plugins/twitter"
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

func init() {
}

func main() {
	// TODO NewAccount called here is just for playing purposes.
	postWatch := make(chan *PostWatch)

	// Initialize i18n
	gettext.SetLocale(gettext.LC_ALL, "")
	gettext.Textdomain("account-polld")
	gettext.BindTextdomain("account-polld", "/usr/share/locale")

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
	accounts.StartGlibMainLoop()
	watcher := accounts.NewWatcher(SERVICETYPE_POLL)
	mgr := make(map[uint]*AccountManager)
L:
	for {
		select {
		case data := <-watcher.C:
			if account, ok := mgr[data.AccountId]; ok {
				if data.Enabled {
					log.Println("New account data for existing account with id", data.AccountId)
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
					plugin = gmail.New(data.AccountId)
				case SERVICENAME_FACEBOOK:
					// This is just stubbed until the plugin exists.
					log.Println("Creating account with id", data.AccountId, "for", data.ServiceName)
					plugin = facebook.New(data.AccountId)
				case SERVICENAME_TWITTER:
					// This is just stubbed until the plugin exists.
					log.Println("Creating account with id", data.AccountId, "for", data.ServiceName)
					plugin = twitter.New()
				default:
					log.Println("Unhandled account with id", data.AccountId, "for", data.ServiceName)
					break L
				}
				mgr[data.AccountId] = NewAccountManager(watcher, postWatch, plugin)
				mgr[data.AccountId].updateAuthData(data)
				go mgr[data.AccountId].Loop()
			}
		case data := <-watcher.AccountCh:
			log.Println("New Account:", data.AccountId, "for", data.ServiceName)
			summary := gettext.Gettext("New account created")
			body := gettext.Gettext("Tap on it to enable notifications")
			epoch := time.Now().Unix()
			action := "settings://personal/online-accounts"
			pushMsg := *plugins.NewStandardPushMessage(summary, body, action, "", epoch)
			msgs := []plugins.PushMessage{pushMsg}
			postWatch <- &PostWatch{messages: msgs, appId: "_ubuntu-system-settings"}
			break L
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
	pkg := POSTAL_OBJECT_PATH_PART
	idParts := strings.Split(string(id), "_")
	if len(idParts) < 2 {
		panic(fmt.Sprintf("APP_ID '%s' is not valid", id))
	} else if strings.HasPrefix(string(id), "_") {
		// it's a legacy app!
		pkg += "_"
	} else {
		for _, c := range idParts[0] {
			switch c {
			case '+', '.', '-', ':', '~', '_':
				pkg += fmt.Sprintf("_%x", string(c))
			default:
				pkg += string(c)
			}
		}
	}
	return dbus.ObjectPath(pkg)
}
