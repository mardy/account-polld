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
	"fmt"
	// "net/http"
	"net/mail"
	// "net/url"
	// "os"
	// "sort"
	"time"

	"bytes"
	"log"
	"strings"

	"launchpad.net/account-polld/accounts"
	// "launchpad.net/account-polld/gettext"
	"launchpad.net/account-polld/plugins"
	"launchpad.net/account-polld/plugins/imap/go-imap/goimap"
	// "launchpad.net/account-polld/qtcontact"
)

const (
	APP_ID = "imap-accounts.nikwen_imap-accounts"
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

type Message struct {
	uid uint32
	sender string
	subject string
	message string
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
	// Get the user's login data
	user := authData.ClientId
	password := authData.ClientSecret

	// Connect to the IMAP server
	var c *goimap.Client
	var err error
	addr := "imap.gmail.com:993"
	if strings.HasSuffix(addr, ":993") {
		c, err = goimap.DialTLS(addr, nil)
	} else {
		c, err = goimap.Dial(addr)
	}
	if err != nil {
		log.Print("imap plugin ", p.accountId, ": failed to dial host: ", err)
		return nil, err
	}

	// Make sure that we log out afterwards
	defer func() {
		_, err := c.Logout(10 * time.Second)
		if err != nil {
			log.Print("imap plugin ", p.accountId, ": failed to log out: ", err)
		}
	}()

	// Enable session privacy protection and integrity checking if available
	if c.Caps["STARTTLS"] {
		_, err := c.StartTLS(nil)
		if err != nil {
			log.Print("imap plugin ", p.accountId, ": failed to start tls: ", err)
			return nil, err
		}
	}

	// Allow the server to send status commands
	_, err = goimap.Wait(c.Noop())
	if err != nil {
		log.Print("imap plugin ", p.accountId, ": error during noop: ", err)
		return nil, err
	}
	c.Data = nil

	// Log in using the user's credentials
	_, err = c.Login(user, password)
	if err != nil {
		log.Print("imap plugin ", p.accountId, ": failed to log in: ", err)
		return nil, err
	}

	// Select the inbox
	_, err = c.Select("INBOX", true)
	if err != nil {
		log.Print("imap plugin ", p.accountId, ": failed to select the inbox: ", err)
		return nil, err
	}
	c.Data = nil

	// Get all uids of unseen mails // TODO: max limit!
	cmd, err := goimap.Wait(c.UIDSearch("1:* UNSEEN"))
	if err != nil {
		log.Print("imap plugin ", p.accountId, ": failed to get unseen messages: ", err)
		return nil, err
	}

	// fetch unread messages by ids
	set, _ := goimap.NewSeqSet("")
	set.AddNum(cmd.Data[0].SearchResults()...)
	cmd, err = c.UIDFetch(set, "RFC822", "UID", "BODY")
	if err != nil {
		log.Print("imap plugin ", p.accountId, ": failed fetch messages by uids: ", err)
		return nil, err
	}

	messages := []*Message{}

	// Process responses while the command is running
	for cmd.InProgress() {
		// Wait for the next response (no timeout)
		c.Recv(-1)

		// Process command data
		for _, rsp := range cmd.Data {
			msgInfo := rsp.MessageInfo()
			body := goimap.AsBytes(msgInfo.Attrs["BODY"])
			if msg, _ := mail.ReadMessage(bytes.NewReader(body)); msg != nil {
				rawAddress := goimap.AsString(msg.Header.Get("From"))
				address, err := mail.ParseAddress(rawAddress)
				var sender string
				if err != nil {
					log.Print("imap plugin ", p.accountId, ": failed to parse email address: ", err)
					sender = rawAddress
				} else if len(address.Name) > 0 {
					sender = address.Name
				} else {
					sender = address.Address
				}

				bodyBuffer := new(bytes.Buffer)
				bodyBuffer.ReadFrom(address.Body)

				messages = append(messages, &Message{
					uid: goimap.AsNumber(msgInfo.Attrs["UID"]),
					sender: sender,
					subject: msg.Header.Get("Subject"),
					message: bodyBuffer.toString(),
				})

				log.Print(fmt.Sprintf("Message: %#v", Message{ // TODO: Remove (debugging only)
					uid: goimap.AsNumber(msgInfo.Attrs["UID"]),
					sender: sender,
					subject: msg.Header.Get("Subject"),
					message: bodyBuffer.toString(),
				}))
			}
		}
		cmd.Data = nil
		c.Data = nil
	}

	// Log the messages
	log.Print(fmt.Sprintf("Message subjects: %v", messages))

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

// func (p *ImapPlugin) createNotifications(messages []Message) ([]*plugins.PushMessage, error) {
// 	timestamp := time.Now()
// 	pushMsgMap := make(map[string]*plugins.PushMessage)
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
