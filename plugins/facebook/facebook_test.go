/*
 Copyright 2014 Canonical Ltd.
 Authors: James Henstridge <james.henstridge@canonical.com>

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
	"bytes"
	"io"
	"net/http"
	"testing"

	. "launchpad.net/gocheck"

	"launchpad.net/account-polld/plugins"
)

type S struct{}

func init() {
	Suite(S{})
}

func TestAll(t *testing.T) {
	TestingT(t)
}

// closeWraper adds a dummy Close() method to a reader
type closeWrapper struct {
	io.Reader
}

func (r closeWrapper) Close() error {
	return nil
}

const (
	errorBody = `
{
  "error": {
    "message": "Unknown path components: /xyz",
    "type": "OAuthException",
    "code": 2500
  }
}`
	tokenExpiredErrorBody = `
{
  "error": {
    "message": "Error validating access token: Session has expired",
    "type": "OAuthException",
    "code": 190 ,
    "error_subcode": 463
  }
}`
	notificationsBody = `
{
  "data": [
    {
      "id": "notif_id",
      "from": {
        "id": "sender_id",
        "name": "Sender"
      },
      "to": {
        "id": "recipient_id",
        "name": "Recipient"
      },
      "created_time": "2014-07-12T09:51:57+0000",
      "updated_time": "2014-07-12T09:51:57+0000",
      "title": "Sender posted on your timeline: \"The message...\"",
      "link": "http://www.facebook.com/recipient/posts/id",
      "application": {
        "name": "Wall",
        "namespace": "wall",
        "id": "2719290516"
      },
      "unread": 1
    },
    {
      "id": "notif_1105650586_80600069",
      "from": {
        "id": "sender2_id",
        "name": "Sender2"
      },
      "to": {
        "id": "recipient_id",
        "name": "Recipient"
      },
      "created_time": "2014-07-08T06:17:52+0000",
      "updated_time": "2014-07-08T06:17:52+0000",
      "title": "Sender2's birthday was on July 7.",
      "link": "http://www.facebook.com/profile.php?id=xxx&ref=brem",
      "application": {
        "name": "Gifts",
        "namespace": "superkarma",
        "id": "329122197162272"
      },
      "unread": 1,
      "object": {
        "id": "sender2_id",
        "name": "Sender2"
      }
    }
  ],
  "paging": {
    "previous": "https://graph.facebook.com/v2.0/recipient/notifications?limit=5000&since=1405158717&__paging_token=enc_AewDzwIQmWOwPNO-36GaZsaJAog8l93HQ7uLEO-gp1Tb6KCiolXfzMCcGY2KjrJJsDJXdDmNJObICr5dewfMZgGs",
    "next": "https://graph.facebook.com/v2.0/recipient/notifications?limit=5000&until=1404705077&__paging_token=enc_Aewlhut5DQyhqtLNr7pLCMlYU012t4XY7FOt7cooz4wsWIWi-Jqz0a0IDnciJoeLu2vNNQkbtOpCmEmsVsN4hkM4"
  },
  "summary": [
  ]
}
`
)

func (s S) TestParseNotifications(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeWrapper{bytes.NewReader([]byte(notificationsBody))},
	}
	p := &fbPlugin{}
	messages, err := p.parseResponse(resp)
	c.Assert(err, IsNil)
	c.Assert(len(messages), Equals, 2)
	c.Check(messages[0].Notification.Card.Summary, Equals, "Sender")
	c.Check(messages[0].Notification.Card.Body, Equals, "Sender posted on your timeline: \"The message...\"")
	c.Check(messages[1].Notification.Card.Summary, Equals, "Sender2")
	c.Check(messages[1].Notification.Card.Body, Equals, "Sender2's birthday was on July 7.")
	c.Check(p.lastUpdate, Equals, timeStamp("2014-07-12T09:51:57+0000"))
}

func (s S) TestIgnoreOldNotifications(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeWrapper{bytes.NewReader([]byte(notificationsBody))},
	}
	p := &fbPlugin{lastUpdate: "2014-07-08T06:17:52+0000"}
	messages, err := p.parseResponse(resp)
	c.Assert(err, IsNil)
	c.Assert(len(messages), Equals, 1)
	c.Check(messages[0].Notification.Card.Summary, Equals, "Sender")
	c.Check(messages[0].Notification.Card.Body, Equals, "Sender posted on your timeline: \"The message...\"")
	c.Check(p.lastUpdate, Equals, timeStamp("2014-07-12T09:51:57+0000"))
}

func (s S) TestErrorResponse(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       closeWrapper{bytes.NewReader([]byte(errorBody))},
	}
	p := &fbPlugin{}
	notifications, err := p.parseResponse(resp)
	c.Check(notifications, IsNil)
	c.Assert(err, Not(IsNil))
	graphErr := err.(*GraphError)
	c.Check(graphErr.Message, Equals, "Unknown path components: /xyz")
	c.Check(graphErr.Code, Equals, 2500)
}

func (s S) TestTokenExpiredErrorResponse(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       closeWrapper{bytes.NewReader([]byte(tokenExpiredErrorBody))},
	}
	p := &fbPlugin{}
	notifications, err := p.parseResponse(resp)
	c.Check(notifications, IsNil)
	c.Assert(err, Equals, plugins.ErrTokenExpired)
}
