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
	"fmt"

	"launchpad.net/account-polld/plugins"
)

type pushes map[string]plugins.PushMessage

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
	// Snippet is a short part of the message text. This text is
	// used for the push message summary.
	Snippet string `json:"snippet"`
	// Payload represents the message payload.
	Payload payload `json:"payload"`
}

// payload represents the message payload.
type payload struct {
	Headers []messageHeader `json:"headers"`
}

func (p *payload) mapHeaders() map[string]string {
	headers := make(map[string]string)
	for _, hdr := range p.Headers {
		headers[hdr.Name] = hdr.Value
	}
	return headers
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
	return fmt.Sprintln("backend response:", err.Err.Message)
}

const (
	hdr_DATE    = "Date"
	hdr_SUBJECT = "Subject"
	hdr_FROM    = "From"
)
