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

#include "push_client.h"

#include "debug.h"

#include <QByteArray>
#include <QDBusConnection>
#include <QDBusMessage>
#include <QJsonDocument>
#include <QJsonObject>

using namespace AccountPolld;

namespace AccountPolld {

class PushClientPrivate: public QObject
{
    Q_OBJECT
    Q_DECLARE_PUBLIC(PushClient)

public:
    PushClientPrivate(PushClient *q);
    ~PushClientPrivate() {};

private:
    QDBusConnection m_conn;
    PushClient *q_ptr;
};

} // namespace

PushClientPrivate::PushClientPrivate(PushClient *q):
    QObject(q),
    m_conn(QDBusConnection::sessionBus()),
    q_ptr(q)
{
}

PushClient::PushClient(QObject *parent):
    QObject(parent),
    d_ptr(new PushClientPrivate(this))
{
}

PushClient::~PushClient()
{
    delete d_ptr;
}

void PushClient::post(const QString &appId, const QJsonObject &message)
{
    Q_D(PushClient);

    QDBusMessage msg = QDBusMessage::createMethodCall("com.ubuntu.Postal",
                                                      "/com/ubuntu/Postal",
                                                      "com.ubuntu.Postal",
                                                      "Post");
    msg << appId;
    QByteArray data = QJsonDocument(message).toJson(QJsonDocument::Compact);
    msg << QString::fromUtf8(data);

    d->m_conn.send(msg);
}

#include "push_client.moc"
