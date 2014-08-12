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

package gmail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"sort"
	"time"

	"log"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/gettext"
	"launchpad.net/account-polld/plugins"
)

const (
	APP_ID           = "com.ubuntu.developer.webapps.webapp-gmail_webapp-gmail"
	gmailDispatchUrl = "https://mail.google.com/mail/mu/mp/#cv/priority/^smartlabel_%s/%s"
	// this means 3 individual messages + 1 bundled notification.
	individualNotificationsLimit = 2
	pluginName                   = "gmail"
)

type reportedIdMap map[string]time.Time

var baseUrl, _ = url.Parse("https://www.googleapis.com/gmail/v1/users/me/")

// timeDelta defines how old messages can be to be reported.
var timeDelta = time.Duration(time.Hour * 24)

// trackDelta defines how old messages can be before removed from tracking
var trackDelta = time.Duration(time.Hour * 24 * 7)

type GmailPlugin struct {
	// reportedIds holds the messages that have already been notified. This
	// approach is taken against timestamps as it avoids needing to call
	// get on the message.
	reportedIds reportedIdMap
	accountId   uint
}

func idsFromPersist(accountId uint) (ids reportedIdMap, err error) {
	err = plugins.FromPersist(pluginName, accountId, &ids)
	if err != nil {
		return nil, err
	}
	// discard old ids
	timestamp := time.Now()
	for k, v := range ids {
		delta := timestamp.Sub(v)
		if delta > trackDelta {
			log.Print("gmail plugin ", accountId, ": deleting ", k, " as ", delta, " is greater than ", trackDelta)
			delete(ids, k)
		}
	}
	return ids, nil
}

func (ids reportedIdMap) persist(accountId uint) (err error) {
	err = plugins.Persist(pluginName, accountId, ids)
	if err != nil {
		log.Print("gmail plugin ", accountId, ": failed to save state: ", err)
	}
	return nil
}

func New(accountId uint) *GmailPlugin {
	reportedIds, err := idsFromPersist(accountId)
	if err != nil {
		log.Print("gmail plugin ", accountId, ": cannot load previous state from storage: ", err)
	} else {
		log.Print("gmail plugin ", accountId, ": last state loaded from storage")
	}
	return &GmailPlugin{reportedIds: reportedIds, accountId: accountId}
}

func (p *GmailPlugin) ApplicationId() plugins.ApplicationId {
	return plugins.ApplicationId(APP_ID)
}

func (p *GmailPlugin) Poll(authData *accounts.AuthData) ([]plugins.PushMessage, error) {
	// This envvar check is to ease testing.
	if token := os.Getenv("ACCOUNT_POLLD_TOKEN_GMAIL"); token != "" {
		authData.AccessToken = token
	}

	resp, err := p.requestMessageList(authData.AccessToken)
	if err != nil {
		return nil, err
	}
	messages, err := p.parseMessageListResponse(resp)
	if err != nil {
		return nil, err
	}

	// TODO use the batching API defined in https://developers.google.com/gmail/api/guides/batch
	for i := range messages {
		resp, err := p.requestMessage(messages[i].Id, authData.AccessToken)
		if err != nil {
			return nil, err
		}
		messages[i], err = p.parseMessageResponse(resp)
		if err != nil {
			return nil, err
		}
	}
	return p.createNotifications(messages)
}

func (p *GmailPlugin) reported(id string) bool {
	_, ok := p.reportedIds[id]
	return ok
}

func (p *GmailPlugin) createNotifications(messages []message) ([]plugins.PushMessage, error) {
	timestamp := time.Now()
	pushMsgMap := make(pushes)

	for _, msg := range messages {
		hdr := msg.Payload.mapHeaders()

		from := hdr[hdrFROM]
		if emailAddress, err := mail.ParseAddress(hdr[hdrFROM]); err == nil {
			if emailAddress.Name != "" {
				from = emailAddress.Name
			}
		}
		msgStamp := hdr.getTimestamp()

		if _, ok := pushMsgMap[msg.ThreadId]; ok {
			// TRANSLATORS: the %s is an appended "from" corresponding to an specific email thread
			pushMsgMap[msg.ThreadId].Notification.Card.Summary += fmt.Sprintf(gettext.Gettext(", %s"), from)
		} else if timestamp.Sub(msgStamp) < timeDelta {
			// TRANSLATORS: the %s is the "from" header corresponding to a specific email
			summary := fmt.Sprintf(gettext.Gettext("%s"), from)
			// TRANSLATORS: the first %s refers to the email "subject", the second %s refers "from"
			body := fmt.Sprintf(gettext.Gettext("%s\n%s"), hdr[hdrSUBJECT], msg.Snippet)
			// fmt with label personal and threadId
			action := fmt.Sprintf(gmailDispatchUrl, "personal", msg.ThreadId)
			epoch := hdr.getEpoch()
			pushMsgMap[msg.ThreadId] = *plugins.NewStandardPushMessage(summary, body, action, "", epoch)
		} else {
			log.Print("gmail plugin ", p.accountId, ": skipping message id ", msg.Id, " with date ", msgStamp, " older than ", timeDelta)
		}
	}
	var pushMsg []plugins.PushMessage
	for _, v := range pushMsgMap {
		pushMsg = append(pushMsg, v)
		if len(pushMsg) == individualNotificationsLimit {
			break
		}
	}
	if len(pushMsgMap) > individualNotificationsLimit {
		// TRANSLATORS: This represents a notification summary about more unread emails
		summary := gettext.Gettext("More unread emails available")
		// TODO it would probably be better to grab the estimate that google returns in the message list.
		approxUnreadMessages := len(pushMsgMap) - individualNotificationsLimit
		// TRANSLATORS: the first %d refers to approximate additionl email message count
		body := fmt.Sprintf(gettext.Gettext("You have an approximate of %d additional unread messages"), approxUnreadMessages)
		// fmt with label personal and no threadId
		action := fmt.Sprintf(gmailDispatchUrl, "personal")
		epoch := time.Now().Unix()
		pushMsg = append(pushMsg, *plugins.NewStandardPushMessage(summary, body, action, "", epoch))
	}

	return pushMsg, nil
}

func (p *GmailPlugin) parseMessageListResponse(resp *http.Response) ([]message, error) {
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp errorResp
		if err := decoder.Decode(&errResp); err != nil {
			return nil, err
		}
		if errResp.Err.Code == 401 {
			return nil, plugins.ErrTokenExpired
		}
		return nil, &errResp
	}

	var messages messageList
	if err := decoder.Decode(&messages); err != nil {
		return nil, err
	}

	filteredMsg := p.messageListFilter(messages.Messages)

	return filteredMsg, nil
}

// messageListFilter returns a subset of unread messages where the subset
// depends on not being in reportedIds. Before returning, reportedIds is
// updated with the new list of unread messages.
func (p *GmailPlugin) messageListFilter(messages []message) []message {
	sort.Sort(byId(messages))
	var reportMsg []message
	var ids = make(reportedIdMap)

	for _, msg := range messages {
		if !p.reported(msg.Id) {
			reportMsg = append(reportMsg, msg)
		}
		ids[msg.Id] = time.Now()
	}
	p.reportedIds = ids
	p.reportedIds.persist(p.accountId)
	return reportMsg
}

func (p *GmailPlugin) parseMessageResponse(resp *http.Response) (message, error) {
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp errorResp
		if err := decoder.Decode(&errResp); err != nil {
			return message{}, err
		}
		return message{}, &errResp
	}

	var msg message
	if err := decoder.Decode(&msg); err != nil {
		return message{}, err
	}

	return msg, nil
}

func (p *GmailPlugin) requestMessage(id, accessToken string) (*http.Response, error) {
	u, err := baseUrl.Parse("messages/" + id)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	// only request specific fields
	query.Add("fields", "snippet,threadId,id,payload/headers")
	// get the full message to get From and Subject from headers
	query.Add("format", "full")
	u.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	return http.DefaultClient.Do(req)
}

func (p *GmailPlugin) requestMessageList(accessToken string) (*http.Response, error) {
	u, err := baseUrl.Parse("messages")
	if err != nil {
		return nil, err
	}

	query := u.Query()

	// only get unread, from the personal category that are in the inbox.
	// if we want to widen the search scope we need to add more categories
	// like: '(category:personal or category:updates or category:forums)' ...
	query.Add("q", "is:unread category:personal in:inbox")
	u.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	return http.DefaultClient.Do(req)
}
