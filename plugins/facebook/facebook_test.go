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
package facebook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	"net/http/httptest"
	"testing"

	. "launchpad.net/gocheck"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/account-polld/plugins"
	"launchpad.net/go-xdg/v0"
)

type S struct {
	tempDir string
	ts      *httptest.Server
}

var _ = Suite(&S{})

func Test(t *testing.T) { TestingT(t) }

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
	largeNotificationsBody = `
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
    },
    {
      "id": "notif_id_3",
      "from": {
        "id": "sender3_id",
        "name": "Sender3"
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
      "id": "notif_id_4",
      "from": {
        "id": "sender4_id",
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
      "unread": 1
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

	inboxBody = `
{
  "data": [
    {
      "unread": 1, 
      "unseen": 1, 
      "id": "445809168892281", 
      "updated_time": "2014-08-25T18:39:32+0000", 
      "comments": {
        "data": [
          {
            "id": "445809168892281_1408991972", 
            "from": {
              "id": "346217352202239", 
              "name": "Pollod Magnifico"
            }, 
            "message": "Hola mundo!", 
            "created_time": "2014-08-25T18:39:32+0000"
          }
        ], 
        "paging": {
          "previous": "https://graph.facebook.com/v2.0/445809168892281/comments?limit=1&since=1408991972&__paging_token=enc_Aew2kKJXEXzdm9k89DvLYz_y8nYxUbvElWcn6h_pKMRsoAPTPpkU7-AsGhkcYF6M1qbomOnFJf9ckL5J3hTltLFq", 
          "next": "https://graph.facebook.com/v2.0/445809168892281/comments?limit=1&until=1408991972&__paging_token=enc_Aewlixpk4h4Vq79-W1ixrTM6ONbsMUDrcj0vLABs34tbhWarfpQLf818uoASWNDEpQO4XEXh5HbgHpcCqnuNVEOR"
        }
      }
    },
    {
      "unread": 2, 
      "unseen": 1, 
      "id": "445809168892282", 
      "updated_time": "2014-08-25T18:39:32+0000", 
      "comments": {
        "data": [
          {
            "id": "445809168892282_1408991973", 
            "from": {
              "id": "346217352202239", 
              "name": "Pollitod Magnifico"
            }, 
            "message": "Hola!", 
            "created_time": "2014-08-25T18:39:32+0000"
          }
        ], 
        "paging": {
          "previous": "https://graph.facebook.com/v2.0/445809168892281/comments?limit=1&since=1408991972&__paging_token=enc_Aew2kKJXEXzdm9k89DvLYz_y8nYxUbvElWcn6h_pKMRsoAPTPpkU7-AsGhkcYF6M1qbomOnFJf9ckL5J3hTltLFq", 
          "next": "https://graph.facebook.com/v2.0/445809168892281/comments?limit=1&until=1408991972&__paging_token=enc_Aewlixpk4h4Vq79-W1ixrTM6ONbsMUDrcj0vLABs34tbhWarfpQLf818uoASWNDEpQO4XEXh5HbgHpcCqnuNVEOR"
        }
      }
    },
    {
      "unread": 2, 
      "unseen": 1, 
      "id": "445809168892283", 
      "updated_time": "2014-08-25T18:39:32+0000", 
      "comments": {
        "data": [
          {
            "id": "445809168892282_1408991973", 
            "from": {
              "id": "346217352202240", 
              "name": "A Friend"
            }, 
            "message": "mellon", 
            "created_time": "2014-08-25T18:39:32+0000"
          }
        ], 
        "paging": {
          "previous": "https://graph.facebook.com/v2.0/445809168892281/comments?limit=1&since=1408991972&__paging_token=enc_Aew2kKJXEXzdm9k89DvLYz_y8nYxUbvElWcn6h_pKMRsoAPTPpkU7-AsGhkcYF6M1qbomOnFJf9ckL5J3hTltLFq", 
          "next": "https://graph.facebook.com/v2.0/445809168892281/comments?limit=1&until=1408991972&__paging_token=enc_Aewlixpk4h4Vq79-W1ixrTM6ONbsMUDrcj0vLABs34tbhWarfpQLf818uoASWNDEpQO4XEXh5HbgHpcCqnuNVEOR"
        }
      }
    }


  ], 
  "paging": {
    "previous": "https://graph.facebook.com/v2.0/270128826512416/inbox?fields=unread,unseen,comments.limit(1)&limit=25&since=1408991972&__paging_token=enc_Aey99ACSOyZqN_7I-yWLnY8K3dqu4wVsx-Th3kMHMTMQ5VPbQRPgCQiJps0II1QAXDAVzHplqPS8yNgq8Zs_G2aK", 
    "next": "https://graph.facebook.com/v2.0/270128826512416/inbox?fields=unread,unseen,comments.limit(1)&limit=25&until=1408991972&__paging_token=enc_AewjHkk10NNjRCXJCoaP5hyf22kw-htwxsDaVOiLY-IiXxB99sKNGlfFFmkcG-VeMGUETI2agZGR_1IWP5W4vyPL"
  }, 
  "summary": {
    "unseen_count": 0, 
    "unread_count": 1, 
    "updated_time": "2014-08-25T19:05:49+0000"
  }
}
`
)

func (s *S) SetUpTest(c *C) {
	s.tempDir = c.MkDir()
	plugins.XdgDataFind = func(a string) (string, error) {
		return filepath.Join(s.tempDir, a), nil
	}
	plugins.XdgDataEnsure = func(a string) (string, error) {
		p := filepath.Join(s.tempDir, a)
		base := path.Dir(p)
		if _, err := os.Stat(base); err != nil {
			os.MkdirAll(base, 0700)
		}
		return p, nil
	}
	s.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	baseUrl, _ = url.Parse(s.ts.URL)
}

func (s *S) TearDownTest(c *C) {
	plugins.XdgDataFind = xdg.Data.Find
	plugins.XdgDataEnsure = xdg.Data.Find
	doRequest = request
	s.ts.Close()
}

func (s *S) TestParseNotifications(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeWrapper{bytes.NewReader([]byte(notificationsBody))},
	}
	p := &fbPlugin{}
	batch, err := p.parseResponse(resp)
	c.Assert(err, IsNil)
	c.Assert(batch, NotNil)
	messages := batch.Messages
	c.Assert(len(messages), Equals, 2)
	c.Check(messages[0].Notification.Card.Summary, Equals, "Sender")
	c.Check(messages[0].Notification.Card.Body, Equals, "Sender posted on your timeline: \"The message...\"")
	c.Check(messages[1].Notification.Card.Summary, Equals, "Sender2")
	c.Check(messages[1].Notification.Card.Body, Equals, "Sender2's birthday was on July 7.")
	c.Check(p.state.LastUpdate, Equals, timeStamp("2014-07-12T09:51:57+0000"))
}

func (s *S) TestParseLotsOfNotifications(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeWrapper{bytes.NewReader([]byte(largeNotificationsBody))},
	}
	p := &fbPlugin{}
	batch, err := p.parseResponse(resp)
	c.Assert(err, IsNil)
	c.Assert(batch, NotNil)
	messages := batch.Messages
	c.Assert(len(messages), Equals, 4)
	c.Check(messages[0].Notification.Card.Summary, Equals, "Sender")
	c.Check(messages[0].Notification.Card.Body, Equals, "Sender posted on your timeline: \"The message...\"")
	c.Check(messages[1].Notification.Card.Summary, Equals, "Sender2")
	c.Check(messages[1].Notification.Card.Body, Equals, "Sender2's birthday was on July 7.")
	ofMsg := batch.OverflowHandler(messages[2:])
	c.Check(ofMsg.Notification.Card.Summary, Equals, "Multiple more notifications")
	c.Check(ofMsg.Notification.Card.Body, Equals, "From Sender3, Sender2")
	c.Check(p.state.LastUpdate, Equals, timeStamp("2014-07-12T09:51:57+0000"))
}

func (s *S) TestIgnoreOldNotifications(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeWrapper{bytes.NewReader([]byte(notificationsBody))},
	}
	p := &fbPlugin{state: fbState{LastUpdate: "2014-07-08T06:17:52+0000"}}
	batch, err := p.parseResponse(resp)
	c.Assert(err, IsNil)
	c.Assert(batch, NotNil)
	messages := batch.Messages
	c.Assert(len(messages), Equals, 1)
	c.Check(messages[0].Notification.Card.Summary, Equals, "Sender")
	c.Check(messages[0].Notification.Card.Body, Equals, "Sender posted on your timeline: \"The message...\"")
	c.Check(p.state.LastUpdate, Equals, timeStamp("2014-07-12T09:51:57+0000"))
}

func (s *S) TestParseResponseErrorResponse(c *C) {
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

func (s *S) TestDecodeResponseErrorResponse(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       closeWrapper{bytes.NewReader([]byte(errorBody))},
	}
	p := &fbPlugin{}
	var result notificationDoc
	err := p.decodeResponse(resp, &result)
	c.Check(result, DeepEquals, notificationDoc{})
	c.Assert(err, Not(IsNil))
	graphErr := err.(*GraphError)
	c.Check(graphErr.Message, Equals, "Unknown path components: /xyz")
	c.Check(graphErr.Code, Equals, 2500)
}

func (s *S) TestDecodeResponseErrorResponseFails(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       closeWrapper{bytes.NewReader([]byte("hola" + errorBody))},
	}
	p := &fbPlugin{}
	var result notificationDoc
	err := p.decodeResponse(resp, &result)
	c.Check(result, DeepEquals, notificationDoc{})
	c.Assert(err, NotNil)
	jsonErr := err.(*json.SyntaxError)
	c.Check(jsonErr.Offset, Equals, int64(1))
}

func (s *S) TestTokenExpiredErrorResponse(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       closeWrapper{bytes.NewReader([]byte(tokenExpiredErrorBody))},
	}
	p := &fbPlugin{}
	notifications, err := p.parseResponse(resp)
	c.Check(notifications, IsNil)
	c.Assert(err, Equals, plugins.ErrTokenExpired)
}

func (s *S) TestParseInbox(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeWrapper{bytes.NewReader([]byte(inboxBody))},
	}
	p := &fbPlugin{}
	batch, err := p.parseInboxResponse(resp)
	c.Assert(err, IsNil)
	c.Assert(batch, NotNil)
	messages := batch.Messages
	c.Assert(len(messages), Equals, 3)
	c.Check(messages[0].Notification.Card.Summary, Equals, "Pollod Magnifico")
	c.Check(messages[0].Notification.Card.Body, Equals, "Hola mundo!")
	c.Check(messages[1].Notification.Card.Summary, Equals, "Pollitod Magnifico")
	c.Check(messages[1].Notification.Card.Body, Equals, "Hola!")

	ofMsg := batch.OverflowHandler(messages[batch.Limit:])

	c.Check(ofMsg.Notification.Card.Summary, Equals, "Multiple more messages")
	c.Check(ofMsg.Notification.Card.Body, Equals, "From A Friend")
	c.Check(p.state.LastInboxUpdate, Equals, timeStamp("2014-08-25T18:39:32+0000"))
}

func (s *S) TestDecodeResponse(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeWrapper{bytes.NewReader([]byte(inboxBody))},
	}
	p := &fbPlugin{}
	var doc inboxDoc
	err := p.decodeResponse(resp, &doc)
	c.Check(err, IsNil)
	c.Check(len(doc.Data), Equals, 3)
}

func (s *S) TestDecodeResponseFails(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeWrapper{bytes.NewReader([]byte("hola" + inboxBody))},
	}
	p := &fbPlugin{}
	var doc inboxDoc
	err := p.decodeResponse(resp, &doc)
	c.Assert(err, NotNil)
	jsonErr := err.(*json.SyntaxError)
	c.Check(jsonErr.Offset, Equals, int64(1))
}

func (s *S) TestFilterNotifications(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeWrapper{bytes.NewReader([]byte(inboxBody))},
	}
	p := &fbPlugin{}
	var doc inboxDoc
	p.decodeResponse(resp, &doc)
	var state fbState
	notifications := p.filterNotifications(&doc, &state.LastInboxUpdate)
	c.Check(notifications, HasLen, 3)
	// check if the lastInboxUpdate is updated
	c.Check(state.LastInboxUpdate, Equals, timeStamp("2014-08-25T18:39:32+0000"))
}

func (s *S) TestBuildPushMessages(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeWrapper{bytes.NewReader([]byte(inboxBody))},
	}
	var state fbState
	p := &fbPlugin{state: state, accountId: 32}
	var doc inboxDoc
	p.decodeResponse(resp, &doc)
	notifications := p.filterNotifications(&doc, &p.state.LastInboxUpdate)
	batch := p.buildPushMessages(notifications, &doc, doc.size(), doc.size())
	c.Assert(batch, NotNil)
	c.Check(batch.Messages, HasLen, doc.size())
}

func (s *S) TestBuildPushMessagesConsolidate(c *C) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeWrapper{bytes.NewReader([]byte(inboxBody))},
	}
	var state fbState
	p := &fbPlugin{state: state, accountId: 32}
	var doc inboxDoc
	p.decodeResponse(resp, &doc)
	notifications := p.filterNotifications(&doc, &p.state.LastInboxUpdate)
	max := doc.size() - 2
	batch := p.buildPushMessages(notifications, &doc, max, max)
	c.Assert(batch, NotNil)
	// we should get all the messages in a batch with Limit=max
	c.Check(batch.Messages, HasLen, len(notifications))
	c.Check(batch.Limit, Equals, max)
}

func (s *S) TestStateFromStorageInitialState(c *C) {
	state, err := stateFromStorage(32)
	c.Check(err, NotNil)
	c.Check(state.LastUpdate, Equals, timeStamp(""))
	c.Check(state.LastInboxUpdate, Equals, timeStamp(""))
}

func (s *S) TestStateFromStoragePersist(c *C) {
	filePath := filepath.Join(s.tempDir, "facebook.test/facebook-32.json")
	state, err := stateFromStorage(32)
	c.Check(err, NotNil)
	state.LastInboxUpdate = timeStamp("2014-08-25T18:39:32+0000")
	state.LastUpdate = timeStamp("2014-08-25T18:39:33+0000")
	err = state.persist(32)
	c.Check(err, IsNil)
	jsonData, err := ioutil.ReadFile(filePath)
	c.Check(err, IsNil)
	var data map[string]string
	json.Unmarshal(jsonData, &data)
	c.Check(data["last_inbox_update"], Equals, "2014-08-25T18:39:32+0000")
	c.Check(data["last_notification_update"], Equals, "2014-08-25T18:39:33+0000")
}

func (s *S) TestStateFromStoragePersistFails(c *C) {
	state := fbState{LastInboxUpdate: "2014-08-25T18:39:32+0000", LastUpdate: "2014-08-25T18:39:33+0000"}
	plugins.XdgDataEnsure = plugins.XdgDataFind
	err := state.persist(32)
	c.Check(err, NotNil)
}

func (s *S) TestStateFromStorage(c *C) {
	state := fbState{LastInboxUpdate: "2014-08-25T18:39:32+0000", LastUpdate: "2014-08-25T18:39:33+0000"}
	err := state.persist(32)
	c.Check(err, IsNil)
	newState, err := stateFromStorage(32)
	c.Check(err, IsNil)
	c.Check(newState.LastUpdate, Equals, timeStamp("2014-08-25T18:39:33+0000"))
	c.Check(newState.LastInboxUpdate, Equals, timeStamp("2014-08-25T18:39:32+0000"))
	// bad format
	state = fbState{LastInboxUpdate: "yesterday", LastUpdate: "2014-08-25T18:39:33+0000"}
	state.persist(32)
	_, err = stateFromStorage(32)
	c.Check(err, NotNil)
	state = fbState{LastInboxUpdate: "2014-08-25T18:39:33+0000", LastUpdate: "today"}
	state.persist(32)
	_, err = stateFromStorage(32)
	c.Check(err, NotNil)
}

func (s *S) TestNew(c *C) {
	state := fbState{LastInboxUpdate: "2014-08-25T18:39:32+0000", LastUpdate: "2014-08-25T18:39:33+0000"}
	state.persist(32)
	p := New(32)
	fb := p.(*fbPlugin)
	c.Check(fb.state, DeepEquals, state)
	// with bad format
	state = fbState{LastInboxUpdate: "hola", LastUpdate: "mundo"}
	state.persist(32)
	p = New(32)
	fb = p.(*fbPlugin)
	c.Check(fb.state, DeepEquals, state)
}

func (s *S) TestApplicationId(c *C) {
	expected := plugins.ApplicationId("com.ubuntu.developer.webapps.webapp-facebook_webapp-facebook")
	p := New(32)
	c.Check(p.ApplicationId(), Equals, expected)
}

func (s *S) TestThreadIsValid(c *C) {
	t := thread{}
	t.Unseen = 1
	t.Unread = 2
	// 10m before Now, the thread should be valid
	t.UpdatedTime = timeStamp(time.Now().Add(-10 * time.Minute).Format(facebookTime))
	tStamp := timeStamp(time.Now().Add(-20 * time.Minute).Format(facebookTime))
	c.Check(t.isValid(tStamp), Equals, true)
	// 2m before Now, the thread should be invalid
	t.UpdatedTime = timeStamp(time.Now().Add(-2 * time.Minute).Format(facebookTime))
	c.Check(t.isValid(tStamp), Equals, false)
	// unseen = 0
	t.Unseen = 0
	t.UpdatedTime = timeStamp(time.Now().Add(-10 * time.Minute).Format(facebookTime))
	c.Check(t.isValid(tStamp), Equals, false)
	// unread = 0, unseen = 1
	t.Unread = 0
	t.Unseen = 1
	c.Check(t.isValid(tStamp), Equals, false)
}

func (s *S) TestGetInbox(c *C) {
	doRequest = func(a *accounts.AuthData, url string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       closeWrapper{bytes.NewReader([]byte(inboxBody))},
		}, nil
	}
	var state fbState
	authData := accounts.AuthData{}
	authData.AccessToken = "foo"
	p := &fbPlugin{state: state, accountId: 32}
	batch, err := p.getInbox(&authData)
	c.Assert(err, IsNil)
	c.Assert(batch, NotNil)
	msgs := batch.Messages
	c.Check(msgs, HasLen, 3)
}

func (s *S) TestGetInboxRequestFails(c *C) {
	doRequest = func(a *accounts.AuthData, url string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       closeWrapper{bytes.NewReader([]byte(""))},
		}, fmt.Errorf("please, fail")
	}
	var state fbState
	authData := accounts.AuthData{}
	authData.AccessToken = "foo"
	p := &fbPlugin{state: state, accountId: 32}
	_, err := p.getInbox(&authData)
	c.Check(err, NotNil)
}

func (s *S) TestGetInboxParseResponseFails(c *C) {
	doRequest = func(a *accounts.AuthData, url string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       closeWrapper{bytes.NewReader([]byte("hola" + inboxBody))},
		}, nil
	}
	var state fbState
	authData := accounts.AuthData{}
	authData.AccessToken = "foo"
	p := &fbPlugin{state: state, accountId: 32}
	_, err := p.getInbox(&authData)
	c.Check(err, NotNil)
}

func (s *S) TestGetNotifications(c *C) {
	doRequest = func(a *accounts.AuthData, url string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       closeWrapper{bytes.NewReader([]byte(notificationsBody))},
		}, nil
	}
	var state fbState
	authData := accounts.AuthData{}
	authData.AccessToken = "foo"
	p := &fbPlugin{state: state, accountId: 32}
	batch, err := p.getNotifications(&authData)
	c.Assert(err, IsNil)
	c.Assert(batch, NotNil)
	msgs := batch.Messages
	c.Check(msgs, HasLen, 2)
}

func (s *S) TestGetNotificationsRequestFails(c *C) {
	doRequest = func(a *accounts.AuthData, url string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       closeWrapper{bytes.NewReader([]byte(""))},
		}, fmt.Errorf("please, fail")
	}
	var state fbState
	authData := accounts.AuthData{}
	authData.AccessToken = "foo"
	p := &fbPlugin{state: state, accountId: 32}
	_, err := p.getNotifications(&authData)
	c.Check(err, NotNil)
}

func (s *S) TestGetNotificationsParseResponseFails(c *C) {
	doRequest = func(a *accounts.AuthData, url string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       closeWrapper{bytes.NewReader([]byte("hola" + notificationsBody))},
		}, nil
	}
	var state fbState
	authData := accounts.AuthData{}
	authData.AccessToken = "foo"
	p := &fbPlugin{state: state, accountId: 32}
	_, err := p.getNotifications(&authData)
	c.Check(err, NotNil)
}

func (s *S) TestPoll(c *C) {
	doRequest = func(a *accounts.AuthData, url string) (*http.Response, error) {
		body := inboxBody
		if url == "me/notifications" {
			body = notificationsBody
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       closeWrapper{bytes.NewReader([]byte(body))},
		}, nil
	}
	var state fbState
	authData := accounts.AuthData{}
	authData.AccessToken = "foo"
	p := &fbPlugin{state: state, accountId: 32}
	batches, err := p.Poll(&authData)
	c.Assert(err, IsNil)
	c.Assert(batches, NotNil)
	c.Assert(batches, HasLen, 2)
	c.Assert(batches[0], NotNil)
	c.Assert(batches[1], NotNil)
	c.Check(batches[0].Tag, Equals, "notification")
	c.Check(batches[0].Messages, HasLen, 2)
	c.Check(batches[1].Tag, Equals, "inbox")
	c.Check(batches[1].Messages, HasLen, 3)
}

func (s *S) TestPollSingleError(c *C) {
	doRequest = func(a *accounts.AuthData, url string) (*http.Response, error) {
		body := inboxBody
		if url == "me/notifications" {
			body = "hola" + notificationsBody
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       closeWrapper{bytes.NewReader([]byte(body))},
		}, nil
	}
	var state fbState
	authData := accounts.AuthData{}
	authData.AccessToken = "foo"
	p := &fbPlugin{state: state, accountId: 32}
	batches, err := p.Poll(&authData)
	c.Assert(err, IsNil)
	c.Assert(batches, HasLen, 1)
	c.Assert(batches[0], NotNil)
	c.Check(batches[0].Messages, HasLen, 3)
}

func (s *S) TestPollError(c *C) {
	doRequest = func(a *accounts.AuthData, url string) (*http.Response, error) {
		body := "hola" + inboxBody
		if url == "me/notifications" {
			body = "hola" + notificationsBody
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       closeWrapper{bytes.NewReader([]byte(body))},
		}, nil
	}
	var state fbState
	authData := accounts.AuthData{}
	authData.AccessToken = "foo"
	p := &fbPlugin{state: state, accountId: 32}
	_, err := p.Poll(&authData)
	c.Check(err, NotNil)
}

func (s *S) TestPollUseEnvToken(c *C) {
	doRequest = func(a *accounts.AuthData, url string) (*http.Response, error) {
		c.Check(a.AccessToken, Equals, "bar")
		return nil, fmt.Errorf("please, fail")
	}
	var state fbState
	authData := accounts.AuthData{}
	authData.AccessToken = "foo"
	os.Setenv("ACCOUNT_POLLD_TOKEN_FACEBOOK", "bar")
	// defer the unset
	defer os.Setenv("ACCOUNT_POLLD_TOKEN_FACEBOOK", "")
	p := &fbPlugin{state: state, accountId: 32}
	_, err := p.Poll(&authData)
	c.Check(err, NotNil)
}

func (s *S) TestRequest(c *C) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()
	baseUrl, _ = url.Parse(ts.URL)
	authData := accounts.AuthData{}
	authData.AccessToken = "foo"
	_, err := request(&authData, "me/notifications")
	c.Check(err, IsNil)
}
