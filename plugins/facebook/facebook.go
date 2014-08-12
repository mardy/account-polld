/*
 Copyright 2014 Canonical Ltd.
 Authors: James Henstridge <james.henstridge@canonical.com>
          Sergio Schvezov <sergio.schvezov@canonical.com>

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

package facebook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"log"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/gettext"
	"launchpad.net/account-polld/plugins"
)

const (
	facebookTime                        = "2006-01-02T15:04:05-0700"
	maxIndividualNotifications          = 4
	consolidatedNotificationsIndexStart = maxIndividualNotifications
	pluginName                          = "facebook"
)

var baseUrl, _ = url.Parse("https://graph.facebook.com/v2.0/")

type timeStamp string

func (stamp timeStamp) persist(accountId uint) (err error) {
	err = plugins.Persist(pluginName, accountId, stamp)
	if err != nil {
		log.Print("facebook plugin", accountId, ": failed to save state: ", err)
	}
	return nil
}

func timeStampFromStorage(accountId uint) (stamp timeStamp, err error) {
	err = plugins.FromPersist(pluginName, accountId, &stamp)
	if err != nil {
		return stamp, err
	}
	if _, err := time.Parse(facebookTime, string(stamp)); err == nil {
		return stamp, err
	}
	return stamp, nil
}

type fbPlugin struct {
	lastUpdate timeStamp
	accountId  uint
}

func New(accountId uint) plugins.Plugin {
	stamp, err := timeStampFromStorage(accountId)
	if err != nil {
		log.Print("facebook plugin ", accountId, ": cannot load previous state from storage: ", err)
	} else {
		log.Print("facebook plugin ", accountId, ": last state loaded from storage")
	}
	return &fbPlugin{lastUpdate: stamp, accountId: accountId}
}

func (p *fbPlugin) ApplicationId() plugins.ApplicationId {
	return "com.ubuntu.developer.webapps.webapp-facebook_webapp-facebook"
}

func (p *fbPlugin) request(authData *accounts.AuthData, path string) (*http.Response, error) {
	// Resolve path relative to Graph API base URL, and add access token
	u, err := baseUrl.Parse(path)
	if err != nil {
		return nil, err
	}
	query := u.Query()
	query.Add("access_token", authData.AccessToken)
	u.RawQuery = query.Encode()

	return http.Get(u.String())
}

func (p *fbPlugin) parseResponse(resp *http.Response) ([]plugins.PushMessage, error) {
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var result errorDoc
		if err := decoder.Decode(&result); err != nil {
			return nil, err
		}
		if result.Error.Code == 190 {
			return nil, plugins.ErrTokenExpired
		}
		return nil, &result.Error
	}

	// TODO: Follow the "paging.next" link if we get more than one
	// page full of notifications.  The default limit seems to be
	// 5000 though, which we are unlikely to hit, since
	// notifications are deleted once read.
	// TODO filter out of date messages before operating
	var result notificationDoc
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}

	var validNotifications []notification
	latestUpdate := p.lastUpdate
	for _, n := range result.Data {
		if n.UpdatedTime <= p.lastUpdate {
			log.Println("facebook plugin: skipping notification", n.Id, "as", n.UpdatedTime, "is older than", p.lastUpdate)
		} else if n.Unread != 1 {
			log.Println("facebook plugin: skipping notification", n.Id, "as it's read:", n.Unread)
		} else {
			log.Println("facebook plugin: valid notification", n.Id, "dated:", n.UpdatedTime, "and read status:", n.Unread)
			validNotifications = append(validNotifications, n)
			if n.UpdatedTime > latestUpdate {
				latestUpdate = n.UpdatedTime
			}
		}
	}
	p.lastUpdate = latestUpdate
	p.lastUpdate.persist(p.accountId)

	pushMsg := []plugins.PushMessage{}
	for _, n := range validNotifications {
		epoch := toEpoch(n.UpdatedTime)
		pushMsg = append(pushMsg, *plugins.NewStandardPushMessage(n.From.Name, n.Title, n.Link, n.picture(), epoch))
		if len(pushMsg) == maxIndividualNotifications {
			break
		}
	}

	// Now we consolidate the remaining statuses
	if len(validNotifications) > len(pushMsg) && len(validNotifications) >= consolidatedNotificationsIndexStart {
		usernamesMap := make(map[string]bool)
		for _, n := range result.Data[consolidatedNotificationsIndexStart:] {
			if _, ok := usernamesMap[n.From.Name]; !ok {
				usernamesMap[n.From.Name] = true
			}
		}
		usernames := []string{}
		for k, _ := range usernamesMap {
			usernames = append(usernames, k)
			// we don't too many usernames listed, this is a hard number
			if len(usernames) > 10 {
				usernames = append(usernames, "...")
				break
			}
		}
		if len(usernames) > 0 {
			// TRANSLATORS: This represents a notification summary about more facebook notifications
			summary := gettext.Gettext("Multiple more notifications")
			// TRANSLATORS: This represents a notification body with the comma separated facebook usernames
			body := fmt.Sprintf(gettext.Gettext("From %s"), strings.Join(usernames, ", "))
			action := "https://m.facebook.com"
			epoch := time.Now().Unix()
			pushMsg = append(pushMsg, *plugins.NewStandardPushMessage(summary, body, action, "", epoch))
		}
	}

	return pushMsg, nil
}

func (p *fbPlugin) Poll(authData *accounts.AuthData) ([]plugins.PushMessage, error) {
	// This envvar check is to ease testing.
	if token := os.Getenv("ACCOUNT_POLLD_TOKEN_FACEBOOK"); token != "" {
		authData.AccessToken = token
	}
	resp, err := p.request(authData, "me/notifications")
	if err != nil {
		return nil, err
	}
	return p.parseResponse(resp)
}

func toEpoch(stamp timeStamp) int64 {
	if t, err := time.Parse(facebookTime, string(stamp)); err == nil {
		return t.Unix()
	}
	return time.Now().Unix()
}

// The notifications response format is described here:
// https://developers.facebook.com/docs/graph-api/reference/v2.0/user/notifications/
type notificationDoc struct {
	Data   []notification `json:"data"`
	Paging struct {
		Previous string `json:"previous"`
		Next     string `json:"next"`
	} `json:"paging"`
}

type notification struct {
	Id          string    `json:"id"`
	From        object    `json:"from"`
	To          object    `json:"to"`
	CreatedTime timeStamp `json:"created_time"`
	UpdatedTime timeStamp `json:"updated_time"`
	Title       string    `json:"title"`
	Link        string    `json:"link"`
	Application object    `json:"application"`
	Unread      int       `json:"unread"`
	Object      object    `json:"object"`
}

func (n notification) picture() string {
	u, err := baseUrl.Parse(fmt.Sprintf("%s/picture", n.From.Id))
	if err != nil {
		log.Println("facebook plugin: cannot get picture for", n.Id)
		return ""
	}
	query := u.Query()
	query.Add("redirect", "true")
	u.RawQuery = query.Encode()
	return u.String()
}

type object struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// The error response format is described here:
// https://developers.facebook.com/docs/graph-api/using-graph-api/v2.0#errors
type errorDoc struct {
	Error GraphError `json:"error"`
}

type GraphError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code"`
	Subcode int    `json:"error_subcode"`
}

func (err *GraphError) Error() string {
	return err.Message
}
