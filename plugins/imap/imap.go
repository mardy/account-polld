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
	"bytes"
	"fmt"
	"log"
	"math"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	// "net/url"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/gettext"
	"launchpad.net/account-polld/plugins"
	"launchpad.net/account-polld/plugins/imap/go-imap/goimap"
	"launchpad.net/account-polld/plugins/imap/goenmime"
	"launchpad.net/account-polld/qtcontact"
)

const (
	imapMessageDispatchUri = "imap://%d/uid/%d"
	imapOverflowDispatchUri = "imap://%d"
	// this means 2 individual messages + 1 bundled notification.
	individualNotificationsLimit = 2
	pluginName                   = "imap"
)

type reportedIdMap map[string]time.Time

// Type for sorting an []uint32 slice
type Uint32Slice []uint32

func (p Uint32Slice) Len() int           { return len(p) }
func (p Uint32Slice) Less(i, j int) bool { return p[i] < p[j] }
func (p Uint32Slice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// timeDelta defines how old messages can be to be reported.
var timeDelta = time.Duration(time.Hour * 24)

// trackDelta defines how old messages can be before removed from tracking
var trackDelta = time.Duration(time.Hour * 24 * 7)

type ImapPlugin struct {
	// the app id of the app to display notifications for
	appId       string
	// reportedIds holds the messages that have already been notified. This
	// approach is taken against timestamps as it avoids needing to call
	// get on the message.
	reportedIds reportedIdMap
	accountId   uint
}

type Message struct {
	uid     uint32
	date    time.Time
	from    string
	subject string
	message string
}

func idsFromPersist(accountId uint) (ids reportedIdMap, err error) {
	err = plugins.FromPersist(pluginName, accountId, &ids)
	if err != nil {
		return nil, err
	}
	// Discard old ids
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

func (p *ImapPlugin) reported(id string) bool {
	_, ok := p.reportedIds[id]
	return ok
}

func New(appId string, accountId uint) *ImapPlugin {
	reportedIds, err := idsFromPersist(accountId)
	if err != nil {
		log.Print("imap plugin ", accountId, ": cannot load previous state from storage: ", err)
	} else {
		log.Print("imap plugin ", accountId, ": last state loaded from storage")
	}
	return &ImapPlugin{appId: appId, reportedIds: reportedIds, accountId: accountId}
}

func (p *ImapPlugin) ApplicationId() plugins.ApplicationId {
	return plugins.ApplicationId(p.appId)
}

// TODO: Poll seems to hang when restarting the push client when we have unread emails
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

	log.Print("imap plugin: login")

	// Select the inbox
	_, err = c.Select("INBOX", true)
	if err != nil {
		log.Print("imap plugin ", p.accountId, ": failed to select the inbox: ", err)
		return nil, err
	}
	c.Data = nil

	log.Print("imap plugin: inbox")

	// Get all uids of unseen mails
	cmd, err := goimap.Wait(c.UIDSearch("1:* UNSEEN")) // TODO: Use the SINCE command to filter out emails which are older than a day (and add a notice to the timeDelta declaration)
	if err != nil {
		log.Print("imap plugin ", p.accountId, ": failed to get unseen messages: ", err)
		return nil, err
	}

	log.Print("imap plugin: uidsearch")

	// Filter for those unread messages for which we haven't requested information from the server yet
	unseenUids := cmd.Data[0].SearchResults()
	newUids, uidsToReport := p.uidFilter(unseenUids)

	messages := []*Message{}

	log.Print("imap plugin: before if")

	if len(newUids) > 0 {
		// TODO: Fetch the bodies of the 3 most recent unread messages by their uids (we do not display more than 3 anyway) and create dummy messages for the other ones?
		set, _ := goimap.NewSeqSet("")
		set.AddNum(newUids...) // set.AddNum(newUids[Math.max(len(newUids)-(individualNotificationsLimit+1), 0):]...)
		cmd, err = c.UIDFetch(set, "RFC822", "UID", "BODY[]")
		if err != nil {
			log.Print("imap plugin ", p.accountId, ": failed fetch messages by uids: ", err)
			return nil, err
		}

		log.Print("imap plugin: uidfetch")

		// Process responses while the command is running
		for cmd.InProgress() {
			// Wait for the next response (no timeout)
			c.Recv(-1)

			log.Print("imap plugin: inprogress")

			// Process command data
			for _, rsp := range cmd.Data {
				log.Print("imap plugin: for")

				msgInfo := rsp.MessageInfo()
				body := goimap.AsBytes(msgInfo.Attrs["BODY[]"])
				if msg, err := mail.ReadMessage(bytes.NewReader(body)); msg != nil {
					log.Print("imap plugin: if2")

					from := goimap.AsString(msg.Header.Get("From"))

					date, err := msg.Header.Date()
					if err != nil {
						log.Print("imap plugin ", p.accountId, ": failed to get date from message header: ", err)
					}

					// Parse the message to retrieve its body as plain text (especially needed for multipart messages)
					var message string
					mimeBody, err := goenmime.ParseMIMEBody(msg)
					if err != nil {
						log.Print("imap plugin ", p.accountId, ": failed to parse mime body: ", err)
					} else {
						message = mimeBody.Text
					}

					messages = append(messages, &Message{
						uid:     goimap.AsNumber(msgInfo.Attrs["UID"]),
						date:    date,
						from:    from,
						subject: mimeBody.GetHeader("Subject"),
						message: message,
					})

					log.Print("imap plugin: append")
				} else if err != nil {
					log.Print("imap plugin ", p.accountId, ": failed to parse message body: ", err)
				}
			}
			cmd.Data = nil
			c.Data = nil
		}

		// Report uids after polling succeeded
		p.reportedIds = uidsToReport
		p.reportedIds.persist(p.accountId)
	} else {
		log.Print("imap plugin: else")
	}

	notif := p.createNotifications(messages)

	log.Print("imap plugin: createNotifications")

	return []*plugins.PushMessageBatch{
		&plugins.PushMessageBatch{
			Messages:        notif,
			Limit:           individualNotificationsLimit,
			OverflowHandler: p.handleOverflow,
			Tag:             "imap",
		}}, nil
}

func (p *ImapPlugin) createNotifications(messages []*Message) []*plugins.PushMessage {
	timestamp := time.Now()
	pushMsg := make([]*plugins.PushMessage, 0)

	for _, msg := range messages {
		// Parse the message's raw email address
		address, err := mail.ParseAddress(msg.from)

		// Get the sender's name
		var sender string
		if err != nil {
			log.Print("imap plugin ", p.accountId, ": failed to parse email address: ", err)
			sender = msg.from
		} else if len(address.Name) > 0 {
			sender = address.Name
		} else {
			sender = address.Address
		}

		// Get the sender's avatar if the email address is in the user's contacts list
		var avatarPath string
		if len(address.Address) > 0 {
			avatarPath = qtcontact.GetAvatar(address.Address)
		}

		if timestamp.Sub(msg.date) < timeDelta {
			// Remove unnecessary spaces from the beginning and the end of the message and replace all sequences of whitespaces by a single space character
			message := strings.TrimSpace(msg.message)
			whitespaceRegexp, _ := regexp.Compile("\\s+")
			message = whitespaceRegexp.ReplaceAllString(message, " ")

			summary := sender
			body := fmt.Sprintf("%s\n%s", strings.TrimSpace(msg.subject), message[:int(math.Min(float64(len(message)), 200))]) // We do not need more than 200 characters
			action := fmt.Sprintf(imapMessageDispatchUri, p.accountId, msg.uid)
			epoch := msg.date.Unix()
			pushMsg = append(pushMsg, plugins.NewStandardPushMessage(summary, body, action, avatarPath, epoch))
		} else {
			log.Print("imap plugin ", p.accountId, ": skipping message uid ", msg.uid, " with date ", msg.date, " older than ", timeDelta)
		}
	}

	return pushMsg
}

func (p *ImapPlugin) handleOverflow(pushMsg []*plugins.PushMessage) *plugins.PushMessage {
	// TRANSLATORS: This represents a notification summary about more unread emails
	summary := gettext.Gettext("More unread emails available")
	approxUnreadMessages := len(pushMsg)
	// TRANSLATORS: the first %d refers to approximate additional email message count
	body := fmt.Sprintf(gettext.Gettext("You have about %d more unread messages"), approxUnreadMessages)
	action := fmt.Sprintf(imapOverflowDispatchUri, p.accountId)
	epoch := time.Now().Unix()

	return plugins.NewStandardPushMessage(summary, body, action, "", epoch)
}

// uidFilter filters a list of message uids for those which have not been reported yet.
// It also returns a list of messages which need to be reported and sorts its output.
func (p *ImapPlugin) uidFilter(uids []uint32) (newUids []uint32, uidsToReport reportedIdMap) {
	sort.Sort(Uint32Slice(uids))
	uidsToReport = make(reportedIdMap)

	for _, uid := range uids {
		uidString := strconv.FormatUint(uint64(uid), 10)
		if !p.reported(uidString) {
			newUids = append(newUids, uid)
		}
		uidsToReport[uidString] = time.Now()
	}

	return newUids, uidsToReport
}
