/*
 * Copyright (C) 2016 Canonical Ltd.
 *
 * Contact: Alberto Mardegan <alberto.mardegan@canonical.com>
 *
 * This file is part of account-polld
 *
 * This library is free software; you can redistribute it and/or
 * modify it under the terms of the GNU Lesser General Public License
 * version 2.1 as published by the Free Software Foundation.
 *
 * This library is distributed in the hope that it will be useful, but
 * WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
 * Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public
 * License along with this library; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA
 * 02110-1301 USA
 */

#ifndef AP_FAKE_PUSH_CLIENT_H
#define AP_FAKE_PUSH_CLIENT_H

#include <QVariantMap>
#include <libqtdbusmock/DBusMock.h>

class FakePushClient
{
public:
    FakePushClient(QtDBusMock::DBusMock *mock): m_mock(mock) {
        m_mock->registerTemplate("com.ubuntu.Postal",
                                 PUSH_CLIENT_MOCK_TEMPLATE,
                                 QDBusConnection::SessionBus);
    }

    OrgFreedesktopDBusMockInterface &mockedService() {
        return m_mock->mockInterface("com.ubuntu.Postal",
                                     "/com/ubuntu/Postal",
                                     "com.ubuntu.Postal",
                                     QDBusConnection::SessionBus);
    }

private:
    QtDBusMock::DBusMock *m_mock;
};

#endif // AP_FAKE_PUSH_CLIENT_H
