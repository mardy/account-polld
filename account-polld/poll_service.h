/*
 * Copyright (C) 2016 Canonical Ltd.
 *
 * Contact: Alberto Mardegan <alberto.mardegan@canonical.com>
 *
 * This file is part of account-polld
 *
 * This program is free software: you can redistribute it and/or modify it
 * under the terms of the GNU General Public License version 3, as published
 * by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful, but
 * WITHOUT ANY WARRANTY; without even the implied warranties of
 * MERCHANTABILITY, SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR
 * PURPOSE.  See the GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along
 * with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

#ifndef AP_POLL_SERVICE_H
#define AP_POLL_SERVICE_H

#include <QDBusContext>
#include <QDBusMessage>
#include <QObject>

namespace AccountPolld {

#define ACCOUNT_POLLD_OBJECT_PATH \
    QStringLiteral("/com/ubuntu/AccountPolld")
#define ACCOUNT_POLLD_SERVICE_NAME \
    QStringLiteral("com.ubuntu.AccountPolld")

class PollServicePrivate;

class PollService: public QObject, protected QDBusContext
{
    Q_OBJECT
    Q_CLASSINFO("D-Bus Interface", "com.ubuntu.AccountPolld")
    Q_CLASSINFO("D-Bus Introspection", ""
"  <interface name=\"com.ubuntu.AccountPolld\">\n"
"    <method name=\"Poll\" />\n"
"    <signal name=\"Done\" />\n"
"  </interface>\n"
        "")

public:
    explicit PollService(QObject *parent = 0);
    ~PollService();

public Q_SLOTS:
    void Poll();

Q_SIGNALS:
    void Done();

private:
    PollServicePrivate *d_ptr;
    Q_DECLARE_PRIVATE(PollService)
};

} // namespace

#endif // AP_POLL_SERVICE_H
