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

package dekko

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"log"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/gettext"
	"launchpad.net/account-polld/plugins"
	"launchpad.net/account-polld/qtcontact"
)

const (
	APP_ID           = "dekko.dekkoproject_dekko"
	dekkoDispatchUrl = "dekko://notify/%d/%s/%s"
	// If there's more than 10 emails in one batch, we don't show 10 notification
	// bubbles, but instead show one summary. We always show all notifications in the
	// indicator.
	individualNotificationsLimit = 10
	pluginName                   = "dekko"
)

type reportedIdMap map[string]time.Time

var baseUrl, _ = url.Parse("https://www.googleapis.com/gmail/v1/users/me/")

// timeDelta defines how old messages can be to be reported.
var timeDelta = time.Duration(time.Hour * 24)

// trackDelta defines how old messages can be before removed from tracking
var trackDelta = time.Duration(time.Hour * 24 * 7)

// relativeTimeDelta is the same as timeDelta
var relativeTimeDelta string = "1d"

// regexp for identifying non-ascii characters
var nonAsciiChars, _ = regexp.Compile("[^\x00-\x7F]")

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

func (p *GmailPlugin) Poll(authData *accounts.AuthData) ([]*plugins.PushMessageBatch, error) {
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
	notif, err := p.createNotifications(messages)
	if err != nil {
		return nil, err
	}
	return []*plugins.PushMessageBatch{
		&plugins.PushMessageBatch{
			Messages:        notif,
			Limit:           individualNotificationsLimit,
			OverflowHandler: p.handleOverflow,
			Tag:             "dekko",
		}}, nil

}

func (p *GmailPlugin) reported(id string) bool {
	_, ok := p.reportedIds[id]
	return ok
}

func (p *GmailPlugin) createNotifications(messages []message) ([]*plugins.PushMessage, error) {
	timestamp := time.Now()
	pushMsgMap := make(pushes)

	for _, msg := range messages {
		hdr := msg.Payload.mapHeaders()

		from := hdr[hdrFROM]
		var avatarPath string

		emailAddress, err := mail.ParseAddress(from)
		if err != nil {
			// If the email address contains non-ascii characters, we get an
			// error so we're going to try again, this time mangling the name
			// by removing all non-ascii characters. We only care about the email
			// address here anyway.
			// XXX: We can't check the error message due to [1]: the error
			// message is different in go < 1.3 and > 1.5.
			// [1] https://github.com/golang/go/issues/12492
			mangledAddr := nonAsciiChars.ReplaceAllString(from, "")
			mangledEmail, mangledParseError := mail.ParseAddress(mangledAddr)
			if mangledParseError == nil {
				emailAddress = mangledEmail
			}
		} else if emailAddress.Name != "" {
			// We only want the Name if the first ParseAddress
			// call was successful. I.e. we do not want the name
			// from a mangled email address.
			from = emailAddress.Name
		}

		if emailAddress != nil {
			avatarPath = qtcontact.GetAvatar(emailAddress.Address)
			// If icon path starts with a path separator, assume local file path,
			// encode it and prepend file scheme defined in RFC 1738.
			if strings.HasPrefix(avatarPath, string(os.PathSeparator)) {
				avatarPath = url.QueryEscape(avatarPath)
				avatarPath = "file://" + avatarPath
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
			action := fmt.Sprintf(dekkoDispatchUrl, p.accountId, "INBOX", msg.Id)
			epoch := hdr.getEpoch()
			pushMsgMap[msg.ThreadId] = plugins.NewStandardPushMessage(summary, body, action, avatarPath, epoch)
		} else {
			log.Print("gmail plugin ", p.accountId, ": skipping message id ", msg.Id, " with date ", msgStamp, " older than ", timeDelta)
		}
	}
	pushMsg := make([]*plugins.PushMessage, 0, len(pushMsgMap))
	for _, v := range pushMsgMap {
		pushMsg = append(pushMsg, v)
	}
	return pushMsg, nil

}
func (p *GmailPlugin) handleOverflow(pushMsg []*plugins.PushMessage) *plugins.PushMessage {
	// TODO it would probably be better to grab the estimate that google returns in the message list.
	approxUnreadMessages := len(pushMsg)

	// TRANSLATORS: the %d refers to the number of new email messages.
	summary := fmt.Sprintf(gettext.Gettext("You have %d new messages"), approxUnreadMessages)

	body := ""

	// fmt with label personal and no threadId
	action := fmt.Sprintf(dekkoDispatchUrl, p.accountId, "INBOX")
	epoch := time.Now().Unix()

	return plugins.NewStandardPushMessage(summary, body, action, "", epoch)
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

	// get all unread inbox emails received after
	// the last time we checked. If this is the first
	// time we check, get unread emails after timeDelta
	query.Add("q", fmt.Sprintf("is:unread in:inbox newer_than:%s", relativeTimeDelta))
	u.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	return http.DefaultClient.Do(req)
}
