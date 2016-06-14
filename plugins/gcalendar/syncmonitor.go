/*
 Copyright 2016 Canonical Ltd.

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

package gcalendar

import (
	"log"
	"runtime"

	"launchpad.net/go-dbus/v1"
)

const (
	busInterface = "com.canonical.SyncMonitor"
	busPath      = "/com/canonical/SyncMonitor"
	busName      = "com.canonical.SyncMonitor"
)

type SyncMonitor struct {
	conn *dbus.Connection
	obj  *dbus.ObjectProxy
}

func NewSyncMonitor() *SyncMonitor {
	conn, err := dbus.Connect(dbus.SessionBus)
	if err != nil {
		log.Print("Fail to connect with session bus: ", err)
		return nil
	}

	p := &SyncMonitor{
		conn: conn,
		obj:  conn.Object(busName, busPath),
	}
	runtime.SetFinalizer(p, clean)
	return p
}

func clean(p *SyncMonitor) {
	if p.conn != nil {
		p.conn.Close()
	}
}

func (p *SyncMonitor) ListCalendarsByAccount(accountId uint) (calendars []string, err error) {
	message, err := p.obj.Call(busInterface, "listCalendarsByAccount", uint32(accountId))
	if err != nil {
		var calendars []string
		return calendars, err
	} else {
		err = message.Args(&calendars)
		return calendars, err
	}
}

func (p *SyncMonitor) LastSyncDate(accountId uint, source string) (lastSyncDate string, err error) {
	message, err := p.obj.Call(busInterface, "lastSuccessfulSyncDate", uint32(accountId), source)
	if err != nil {
		return "", err
	} else {
		var lastSyncDate string
		err = message.Args(&lastSyncDate)
		return lastSyncDate, err
	}
}

func (p *SyncMonitor) SyncAccount(accountId uint, serviceName string, sources []string) (err error) {
	_, err = p.obj.Call(busInterface, "syncAccount", uint32(accountId), serviceName, sources)
	return err
}

func (p *SyncMonitor) State() (state string, err error) {
	message, err := p.obj.Call(busInterface, "state")
	if err != nil {
		return "", err
	} else {
		err = message.Args(&state)
		return state, err
	}
}

