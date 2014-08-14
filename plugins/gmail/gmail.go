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
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/textproto"
	"net/url"
	"os"
	"sort"
	"strings"
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

	if len(messages) == 0 {
		log.Print("gmail plugin ", p.accountId, ": no messages to fetch")
		return nil, nil
	}

	messages, err = p.getMessageBatch(messages, authData.AccessToken)
	if err != nil {
		return nil, err
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

func (p *GmailPlugin) getMessageBatch(messages []message, accessToken string) ([]message, error) {
	req, err := p.createBatchRequest(messages, accessToken)
	if err != nil {
		return nil, err
	}

	/*
		dump, err := httputil.DumpRequest(req, true)
		if err != nil {
			return nil, err
		}
		fmt.Println(string(dump))
	*/

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	} else if resp.StatusCode == 401 {
		return nil, plugins.ErrTokenExpired
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recieved %d when requesting message batch", resp.StatusCode)
	}

	/*
		respDump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			return nil, err
		}
		fmt.Println(string(respDump))
	*/

	return p.getBatchResponse(resp)
}

func getBoundary(contentType string) (boundary string, err error) {
	if i := strings.Index(contentType, ";"); i == -1 {
		return boundary, errors.New("no boundary in batch response")
	} else if mediaType := strings.TrimSpace(contentType[:i]); mediaType != "multipart/mixed" {
		return boundary, errors.New("unexpected media type in batch response")
	}

	var start, end int
	if i := strings.Index(contentType, "boundary="); i == -1 {
		return boundary, errors.New("no boundary in batch response")
	} else {
		start = i + len("boundary=")
	}
	if i := strings.Index(contentType[start:], ";"); i == -1 {
		end = len(contentType)
	} else {
		end = start + i
	}

	boundary = strings.TrimSpace(contentType[start:end])
	return boundary, nil
}

func (p *GmailPlugin) getBatchResponse(resp *http.Response) (messages []message, err error) {
	/*
		    This is the ideal solution but the boundary has '=' in it and the parsing fails to parse
		    it correctly, e.g.; "multipart/mixed; boundary=batch_Qau-LYGqak0=_AApYZR4VvsU="

			contentType := resp.Header.Get("Content-Type")
			fmt.Println(contentType)
			mediaType, params, err := mime.ParseMediaType(contentType)
			if err != nil {
				fmt.Println(mediaType)
				return nil, err
			}
			if !strings.HasPrefix(mediaType, "multipart/") {
				return nil, fmt.Errorf("wrong mediatype, expected multipart and received %s", mediaType)
			}
	*/
	contentType := resp.Header.Get("Content-Type")
	boundary, err := getBoundary(contentType)
	if err != nil {
		return nil, err
	}
	multipartR := multipart.NewReader(resp.Body, boundary)
	for {
		part, err := multipartR.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		defer part.Close()
		r := bufio.NewReader(part)
		for {
			line, err := r.ReadString('\n')
			if err == io.EOF {
				return nil, errors.New("EOF reached before content")
			}
			if len(strings.TrimSpace(line)) == 0 {
				break
			}
		}
		var msg message
		decoder := json.NewDecoder(r)
		if err := decoder.Decode(&msg); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (p *GmailPlugin) createBatchRequest(messages []message, accessToken string) (req *http.Request, err error) {
	buffer := new(bytes.Buffer)
	multipartW := multipart.NewWriter(buffer)
	for i := range messages {
		u, err := baseUrl.Parse("messages/" + messages[i].Id)
		if err != nil {
			return nil, err
		}

		query := u.Query()
		// only request specific fields
		query.Add("fields", "snippet,threadId,id,payload/headers")
		// get the full message to get From and Subject from headers
		query.Add("format", "full")
		u.RawQuery = query.Encode()

		mh := make(textproto.MIMEHeader)
		mh.Set("Content-Type", "application/http")
		mh.Set("Content-Id", messages[i].Id)

		partW, err := multipartW.CreatePart(mh)
		if err != nil {
			return nil, err
		}
		content := []byte("GET " + u.String() + " HTTP/1.1\r\n")
		partW.Write(content)
	}

	multipartW.Close()
	req, err = http.NewRequest("POST", "https://www.googleapis.com/batch", buffer)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "multipart/mixed; boundary="+multipartW.Boundary())

	return req, nil
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
