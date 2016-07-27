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
	"fmt"
	"time"

	"launchpad.net/account-polld/plugins"
)

const gmailTime = "Mon, 2 Jan 2006 15:04:05 -0700"

type pushes map[string]*plugins.PushMessage
type headers map[string]string

// messageList holds a response to call to Users.messages: list
// defined in https://developers.google.com/gmail/api/v1/reference/users/messages/list
type messageList struct {
	// Messages holds a list of message.
	Messages []message `json:"messages"`
	// NextPageToken is used to retrieve the next page of results in the list.
	NextPageToken string `json:"nextPageToken"`
	// ResultSizeEstimage is the estimated total number of results.
	ResultSizeEstimage uint64 `json:"resultSizeEstimate"`
}

// message holds a partial response for a Users.messages.
// The full definition of a message is defined in
// https://developers.google.com/gmail/api/v1/reference/users/messages#resource
type message struct {
	// Id is the immutable ID of the message.
	Id string `json:"id"`
	// ThreadId is the ID of the thread the message belongs to.
	ThreadId string `json:"threadId"`
	// HistoryId is the ID of the last history record that modified
	// this message.
	HistoryId string `json:"historyId"`
	// Snippet is a short part of the message text. This text is
	// used for the push message summary.
	Snippet string `json:"snippet"`
	// Payload represents the message payload.
	Payload payload `json:"payload"`
}

func (m message) String() string {
	return fmt.Sprintf("Id: %d, snippet: '%s'\n", m.Id, m.Snippet[:10])
}

// ById implements sort.Interface for []message based on
// the Id field.
type byId []message

func (m byId) Len() int           { return len(m) }
func (m byId) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m byId) Less(i, j int) bool { return m[i].Id < m[j].Id }

// payload represents the message payload.
type payload struct {
	Headers []messageHeader `json:"headers"`
}

func (p *payload) mapHeaders() headers {
	headers := make(map[string]string)
	for _, hdr := range p.Headers {
		headers[hdr.Name] = hdr.Value
	}
	return headers
}

func (hdr headers) getTimestamp() time.Time {
	timestamp, ok := hdr[hdrDATE]
	if !ok {
		return time.Now()
	}

	if t, err := time.Parse(gmailTime, timestamp); err == nil {
		return t
	}
	return time.Now()
}

func (hdr headers) getEpoch() int64 {
	return hdr.getTimestamp().Unix()
}

// messageHeader represents the message headers.
type messageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type errorResp struct {
	Err struct {
		Code    uint64 `json:"code"`
		Message string `json:"message"`
		Errors  []struct {
			Domain  string `json:"domain"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"errors"`
	} `json:"error"`
}

func (err *errorResp) Error() string {
	return fmt.Sprint("backend response:", err.Err.Message)
}

const (
	hdrDATE    = "Date"
	hdrSUBJECT = "Subject"
	hdrFROM    = "From"
)
