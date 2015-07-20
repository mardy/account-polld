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

package imap

import (
	// "encoding/json"
	// "fmt"
	// "net/http"
	// "net/mail"
	// "net/url"
	// "os"
	// "sort"
	"time"

	"log"

	"launchpad.net/account-polld/accounts"
	// "launchpad.net/account-polld/gettext"
	"launchpad.net/account-polld/plugins"
	// "launchpad.net/account-polld/qtcontact"
)

const (
	APP_ID          = "imap-accounts.nikwen_imap-accounts"
	// this means 2 individual messages + 1 bundled notification.
	individualNotificationsLimit = 2
	pluginName                   = "imap"
)

type reportedIdMap map[string]time.Time

// timeDelta defines how old messages can be to be reported.
var timeDelta = time.Duration(time.Hour * 24)

// trackDelta defines how old messages can be before removed from tracking
var trackDelta = time.Duration(time.Hour * 24 * 7)

type ImapPlugin struct {
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
			log.Print("imap plugin ", accountId, ": deleting ", k, " as ", delta, " is greater than ", trackDelta)
			delete(ids, k)
		}
	}
	return ids, nil
}

func (ids reportedIdMap) persist(accountId uint) (err error) {
	err = plugins.Persist(pluginName, accountId, ids)
	if err != nil {
		log.Print("imap plugin ", accountId, ": failed to save state: ", err)
	}
	return nil
}

func New(accountId uint) *ImapPlugin {
	reportedIds, err := idsFromPersist(accountId)
	if err != nil {
		log.Print("imap plugin ", accountId, ": cannot load previous state from storage: ", err)
	} else {
		log.Print("imap plugin ", accountId, ": last state loaded from storage")
	}
	return &ImapPlugin{reportedIds: reportedIds, accountId: accountId}
}

func (p *ImapPlugin) ApplicationId() plugins.ApplicationId {
	return plugins.ApplicationId(APP_ID)
}

func (p *ImapPlugin) Poll(authData *accounts.AuthData) ([]*plugins.PushMessageBatch, error) {
	log.Print("imap plugin: polling")
	log.Print("authData: ", authData.AccountId, ", ", authData.ClientId, ", ", authData.ClientSecret)

	return nil, nil

	// This envvar check is to ease testing.
	// if token := os.Getenv("ACCOUNT_POLLD_TOKEN_GMAIL"); token != "" {
	// 	authData.AccessToken = token
	// }
	//
	// resp, err := p.requestMessageList(authData.AccessToken)
	// if err != nil {
	// 	return nil, err
	// }
	// messages, err := p.parseMessageListResponse(resp)
	// if err != nil {
	// 	return nil, err
	// }
	//
	// // TODO use the batching API defined in https://developers.google.com/gmail/api/guides/batch
	// for i := range messages {
	// 	resp, err := p.requestMessage(messages[i].Id, authData.AccessToken)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	messages[i], err = p.parseMessageResponse(resp)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }
	// notif, err := p.createNotifications(messages)
	// if err != nil {
	// 	return nil, err
	// }
	// return []*plugins.PushMessageBatch{
	// 	&plugins.PushMessageBatch{
	// 		Messages:        notif,
	// 		Limit:           individualNotificationsLimit,
	// 		OverflowHandler: p.handleOverflow,
	// 		Tag:             "imap",
	// 	}}, nil
}

func (p *ImapPlugin) reported(id string) bool {
	_, ok := p.reportedIds[id]
	return ok
}

// func (p *ImapPlugin) createNotifications(messages []message) ([]*plugins.PushMessage, error) {
// 	timestamp := time.Now()
// 	pushMsgMap := make(pushes)
//
// 	for _, msg := range messages {
// 		hdr := msg.Payload.mapHeaders()
//
// 		from := hdr[hdrFROM]
// 		var avatarPath string
//
// 		if emailAddress, err := mail.ParseAddress(hdr[hdrFROM]); err == nil {
// 			if emailAddress.Name != "" {
// 				from = emailAddress.Name
// 				avatarPath = qtcontact.GetAvatar(emailAddress.Address)
// 			}
// 		}
// 		msgStamp := hdr.getTimestamp()
//
// 		if _, ok := pushMsgMap[msg.ThreadId]; ok {
// 			// TRANSLATORS: the %s is an appended "from" corresponding to an specific email thread
// 			pushMsgMap[msg.ThreadId].Notification.Card.Summary += fmt.Sprintf(gettext.Gettext(", %s"), from)
// 		} else if timestamp.Sub(msgStamp) < timeDelta {
// 			// TRANSLATORS: the %s is the "from" header corresponding to a specific email
// 			summary := fmt.Sprintf(gettext.Gettext("%s"), from)
// 			// TRANSLATORS: the first %s refers to the email "subject", the second %s refers "from"
// 			body := fmt.Sprintf(gettext.Gettext("%s\n%s"), hdr[hdrSUBJECT], msg.Snippet)
// 			// fmt with label personal and threadId
// 			action := fmt.Sprintf(imapDispatchUrl, "personal", msg.ThreadId)
// 			epoch := hdr.getEpoch()
// 			pushMsgMap[msg.ThreadId] = plugins.NewStandardPushMessage(summary, body, action, avatarPath, epoch)
// 		} else {
// 			log.Print("imap plugin ", p.accountId, ": skipping message id ", msg.Id, " with date ", msgStamp, " older than ", timeDelta)
// 		}
// 	}
// 	pushMsg := make([]*plugins.PushMessage, 0, len(pushMsgMap))
// 	for _, v := range pushMsgMap {
// 		pushMsg = append(pushMsg, v)
// 	}
// 	return pushMsg, nil
//
// }
// func (p *ImapPlugin) handleOverflow(pushMsg []*plugins.PushMessage) *plugins.PushMessage {
// 	// TRANSLATORS: This represents a notification summary about more unread emails
// 	summary := gettext.Gettext("More unread emails available")
// 	// TODO it would probably be better to grab the estimate that google returns in the message list.
// 	approxUnreadMessages := len(pushMsg)
// 	// TRANSLATORS: the first %d refers to approximate additional email message count
// 	body := fmt.Sprintf(gettext.Gettext("You have about %d more unread messages"), approxUnreadMessages)
// 	// fmt with label personal and no threadId
// 	action := fmt.Sprintf(imapDispatchUrl, "personal")
// 	epoch := time.Now().Unix()
//
// 	return plugins.NewStandardPushMessage(summary, body, action, "", epoch)
// }

// messageListFilter returns a subset of unread messages where the subset
// depends on not being in reportedIds. Before returning, reportedIds is
// updated with the new list of unread messages.
// func (p *ImapPlugin) messageListFilter(messages []message) []message {
// 	sort.Sort(byId(messages))
// 	var reportMsg []message
// 	var ids = make(reportedIdMap)
//
// 	for _, msg := range messages {
// 		if !p.reported(msg.Id) {
// 			reportMsg = append(reportMsg, msg)
// 		}
// 		ids[msg.Id] = time.Now()
// 	}
// 	p.reportedIds = ids
// 	p.reportedIds.persist(p.accountId)
// 	return reportMsg
// }