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

package plugins

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"launchpad.net/account-polld/accounts"
	"launchpad.net/go-xdg/v0"
)

func init() {
	cmdName = filepath.Base(os.Args[0])
}

// Plugin is an interface which the plugins will adhere to for the poll
// daemon to interact with.
//
// Poll interacts with the backend service with the means the plugin defines
// and  returns a list of Notifications to send to the Push service. If an
// error occurs and is returned the daemon can decide to throttle the service.
//
// ApplicationId returns the APP_ID of the delivery target for Post Office.
type Plugin interface {
	ApplicationId() ApplicationId
	Poll(*accounts.AuthData) ([]PushMessage, error)
}

// AuthTokens is a map with tokens the plugins are to use to make requests.
type AuthTokens map[string]interface{}

// ApplicationId represents the application id to direct posts to.
// e.g.: com.ubuntu.diaspora_diaspora or com.ubuntu.diaspora_diaspora_1.0
type ApplicationId string

// NewStandardPushMessage creates a base Notification with common
// components (members) setup.
func NewStandardPushMessage(summary, body, action, icon string, epoch int64) *PushMessage {
	pm := &PushMessage{
		Notification: Notification{
			Card: &Card{
				Summary:   summary,
				Body:      body,
				Actions:   []string{action},
				Icon:      icon,
				Timestamp: epoch,
				Popup:     true,
				Persist:   true,
			},
			Sound:   DefaultSound(),
			Vibrate: DefaultVibration(),
			Tag:     cmdName,
		},
	}
	return pm
}

// PushMessage represents a data structure to be sent over to the
// Post Office. It consists of a Notification and a Message.
type PushMessage struct {
	// Message represents a JSON object that is passed as-is to the
	// application
	Message string `json:"message,omitempty"`
	// Notification (optional) describes the user-facing notifications
	// triggered by this push message.
	Notification Notification `json:"notification,omitempty"`
}

// Notification (optional) describes the user-facing notifications
// triggered by this push message.
type Notification struct {
	// Sound (optional) is the path to a sound file which can or
	// cannot be played depending on user preferences.
	Sound string `json:"sound,omitempty"`
	// Card represents a specific bubble to give to the user
	Card *Card `json:"card,omitempty"`
	// Vibrate is the haptic feedback part of a notification.
	Vibrate *Vibrate `json:"vibrate,omitempty"`
	// EmblemCounter represents and application counter hint
	// related to the notification.
	EmblemCounter *EmblemCounter `json:"emblem-counter,omitempty"`
	// Tag represents a tag to identify persistent notifications
	Tag string `json:"tag,omitempty"`
}

// Card is part of a notification and represents the user visible hints for
// a specific notification.
type Card struct {
	// Summary is a required title. The card will not be presented if this is missing.
	Summary string `json:"summary"`
	// Body is the longer text.
	Body string `json:"body,omitempty"`
	// Whether to show a bubble. Users can disable this, and can easily miss
	// them, so don’t rely on it exclusively.
	Popup bool `json:"popup,omitempty"`
	// Actions provides actions for the bubble's snap decissions.
	Actions []string `json:"actions,omitempty"`
	// Icon is a path to an icon to display with the notification bubble.
	Icon string `json:"icon,omitempty"`
	// Whether to show in notification centre.
	Persist bool `json:"persist,omitempty"`
	// Seconds since the unix epoch, useful for persistent cards.
	Timestamp int64 `json:"Timestamp,omitempty"`
}

// Vibrate is part of a notification and represents the user haptic hints
// for a specific notification.
//
// Duration cannot be used together with Pattern and Repeat.
type Vibrate struct {
	// Duration in milliseconds.
	Duration uint `json:"duration,omitempty"`
	// Pattern is a list of on->off durations that create a pattern.
	Pattern []uint `json:"pattern,omitempty"`
	// Repeat is the number of times a Pattern is to be repeated.
	Repeat uint `json:"repeat,omitempty"`
}

// EmblemCounter is part of a notification and represents the application visual
// hints related to a notification.
type EmblemCounter struct {
	// Count is a number to be displayed over the application’s icon in the
	// launcher.
	Count uint `json:"count"`
	// Visible determines if the counter is visible or not.
	Visible bool `json:"visible"`
}

// The constanst defined here determine the polling aggressivenes with the following criteria
// MAXIMUM: calls, health warning
// HIGH: SMS, chat message, new email
// DEFAULT: social media updates
// LOW: software updates, junk email
const (
	PRIORITY_MAXIMUM = 0
	PRIORITY_HIGH
	PRIORITY_DEFAULT
	PRIORITY_LOW
)

const (
	PLUGIN_EMAIL = 0
	PLUGIN_SOCIAL
)

// ErrTokenExpired is the error returned by a plugin to indicate that
// the web service reported that the authentication token has expired.
var ErrTokenExpired = errors.New("Token expired")

var cmdName string

// Persist stores the plugins data in a common location to a json file
// from which it can recover later
func Persist(pluginName string, accountId uint, data interface{}) (err error) {
	var p string
	defer func() {
		if err != nil && p != "" {
			os.Remove(p)
		}
	}()
	p, err = xdg.Data.Ensure(filepath.Join(cmdName, fmt.Sprintf("%s-%d.json", pluginName, accountId)))
	if err != nil {
		return err
	}
	file, err := os.Create(p)
	if err != nil {
		return err
	}
	defer file.Close()
	w := bufio.NewWriter(file)
	defer w.Flush()
	jsonWriter := json.NewEncoder(w)
	if err := jsonWriter.Encode(data); err != nil {
		return err
	}
	return nil
}

// FromPersist restores the plugins data from a common location which
// was stored in a json file
func FromPersist(pluginName string, accountId uint, data interface{}) (err error) {
	if reflect.ValueOf(data).Kind() != reflect.Ptr {
		return errors.New("decode target is not a pointer")
	}
	var p string
	defer func() {
		if err != nil {
			if p != "" {
				os.Remove(p)
			}
		}
	}()
	p, err = xdg.Data.Find(filepath.Join(cmdName, fmt.Sprintf("%s-%d.json", pluginName, accountId)))
	if err != nil {
		return err
	}
	file, err := os.Open(p)
	if err != nil {
		return err
	}
	defer file.Close()
	jsonReader := json.NewDecoder(file)
	if err := jsonReader.Decode(&data); err != nil {
		return err
	}

	return nil
}

// DefaultSound returns the path to the default sound for a Notification
func DefaultSound() string {
	// path is searched within XDG_DATA_DIRS
	return "sounds/ubuntu/notifications/Blip.ogg"
}

// DefaultVibration returns a Vibrate with the default vibration
func DefaultVibration() *Vibrate {
	return &Vibrate{Duration: 200}
}
