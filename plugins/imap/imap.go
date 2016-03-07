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
	"errors"
	"fmt"
	"log"
	"math"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/gettext"
	"launchpad.net/account-polld/plugins"
	"launchpad.net/account-polld/plugins/imap/go-imap/goimap"
	"launchpad.net/account-polld/plugins/imap/goenmime"
	"launchpad.net/account-polld/qtcontact"
)

const (
	imapMessageDispatchUri = "imap://%d/uid/%d" // TODO: Proper URIs
	imapOverflowDispatchUri = "imap://%d"
	// this means 10 individual messages + 1 bundled notification.
	individualNotificationsLimit = 10
	pluginName                   = "imap"
	inboxName                    = "INBOX"
)

// Type for sorting an []uint32 slice
type Uint32Slice []uint32

func (p Uint32Slice) Len() int           { return len(p) }
func (p Uint32Slice) Less(i, j int) bool { return p[i] < p[j] }
func (p Uint32Slice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// timeDelta defines how old messages may be to be shown to the user
var timeDelta = time.Duration(time.Hour * 24)

type ImapPlugin struct {
	accountId   uint
	// the app id to display notifications for
	appId       string
	inboxStatus InboxStatus
	firstPoll   bool
}

type InboxStatus struct {
	UidNext     uint32
	UidValidity uint32
}

type Message struct {
	uid     uint32
	date    time.Time
	from    string
	subject string
	message string
}

func inboxStatusFromPersist(accountId uint) (status *InboxStatus, err error) {
	err = plugins.FromPersist(pluginName, accountId, &status)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func (p *ImapPlugin) persistInboxStatus() (err error) {
	err = plugins.Persist(pluginName, p.accountId, p.inboxStatus)
	if err != nil {
		log.Print("imap plugin ", p.accountId, ": failed to save state: ", err)
	}
	return err
}

func New(appId string, accountId uint) *ImapPlugin {
	inboxStatus, err := inboxStatusFromPersist(accountId)
	if err != nil {
		log.Print("imap plugin ", accountId, ": cannot load previous state from storage: ", err)
		return &ImapPlugin{appId: appId, accountId: accountId, firstPoll: true}
	} else {
		log.Print("imap plugin ", accountId, ": last state loaded from storage")
	}
	return &ImapPlugin{appId: appId, accountId: accountId, inboxStatus: *inboxStatus, firstPoll: false}
}

func (p *ImapPlugin) ApplicationId() plugins.ApplicationId {
	return plugins.ApplicationId(p.appId)
}

func (p *ImapPlugin) Poll(authData *accounts.AuthData) ([]*plugins.PushMessageBatch, error) {
	if authData.Method != "password" { // TODO: Implement SASL authentication
		return nil, errors.New("passed auth data is not of type 'password'")
	}

	// Get the user's login data
	user := authData.Data["UserName"]
	password := authData.Data["Secret"]

	// Connect to the IMAP server
	var c *goimap.Client
	var err error
	addr := "imap.gmail.com:993" // TODO: Get data from plugin
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

	// Get the UIDNEXT and UIDVALIDITY values of the user's inbox
	cmd, err := goimap.Wait(c.Status(inboxName))
	if err != nil {
		log.Print("imap plugin ", p.accountId, ": failed to get mailbox status: ", err)
		return nil, err
	}
	mailboxStatus := cmd.Data[0].MailboxStatus()

	// Check if the UIDVALIDITY and UIDNEXT values have changed
	uidValidityChanged := mailboxStatus.UIDValidity != p.inboxStatus.UidValidity
	uidNextChanged := mailboxStatus.UIDNext != p.inboxStatus.UidNext

	searchCommand := ""

	// Check if there are unred emails
	if mailboxStatus.Unseen > 0 {
		// If UIDVALIDITY has changed or this is the first poll, fetch all unread emails
		// Otherwise, if UIDNEXT has changed, i.e. a new message has arrived, fetch all new unread emails
		if p.firstPoll || uidValidityChanged {
			searchCommand = "UID 1:* UNSEEN"
		} else if uidNextChanged {
			searchCommand = "UID " + strconv.Itoa(int(p.inboxStatus.UidNext)) + ":* UNSEEN"
		}
	}

	// Update our stored UIDVALIDITY and UIDNEXT values and store them on disk
	p.inboxStatus.UidValidity = mailboxStatus.UIDValidity
	p.inboxStatus.UidNext = mailboxStatus.UIDNext
	p.persistInboxStatus()
	p.firstPoll = false

	// Create a slice which we are going to store our messages in
	messages := []*Message{}

	// Only fetch messages if there are new ones on the server
	if searchCommand != "" {
		// Select the inbox
		_, err = c.Select(inboxName, true)
		if err != nil {
			log.Print("imap plugin ", p.accountId, ": failed to select the inbox: ", err)
			return nil, err
		}
		c.Data = nil

		// Get the uids of all new unread messages
		cmd, err := goimap.Wait(c.UIDSearch(searchCommand))
		if err != nil {
			log.Print("imap plugin ", p.accountId, ": failed to get unseen messages: ", err)
			return nil, err
		}
		unseenUids := cmd.Data[0].SearchResults()

		// Sort the uids (ascending)
		sort.Sort(Uint32Slice(unseenUids))

		// Fetch the bodies of these messages
		set, _ := goimap.NewSeqSet("")
		set.AddNum(unseenUids...)
		cmd, err = c.UIDFetch(set, "RFC822", "UID", "BODY[]")
		if err != nil {
			log.Print("imap plugin ", p.accountId, ": failed fetch messages by uids: ", err)
			return nil, err
		}

		// Process responses while the command is running
		for cmd.InProgress() {
			// Wait for the next response (no timeout)
			c.Recv(-1)

			// Process command data
			for _, rsp := range cmd.Data {
				// Read the message
				msgInfo := rsp.MessageInfo()
				body := goimap.AsBytes(msgInfo.Attrs["BODY[]"])
				if msg, err := mail.ReadMessage(bytes.NewReader(body)); msg != nil {
					// Get the sender's address
					from := goimap.AsString(msg.Header.Get("From"))

					// Get the date of the message
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

					// Build our Message object and add it to our slice
					messages = append(messages, &Message{
						uid:     goimap.AsNumber(msgInfo.Attrs["UID"]),
						date:    date,
						from:    from,
						subject: mimeBody.GetHeader("Subject"),
						message: message,
					})
				} else if err != nil {
					log.Print("imap plugin ", p.accountId, ": failed to parse message body: ", err)
				}
			}
			cmd.Data = nil
			c.Data = nil
		}
	}

	notif := p.createNotifications(messages)

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
