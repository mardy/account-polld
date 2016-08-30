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

#include "debug.h"
#include "poll_service.h"

#include <QDBusArgument>
#include <QDBusConnection>
#include <QVariantMap>

using namespace AccountPolld;

namespace AccountPolld {

class PollServicePrivate: public QObject
{
    Q_OBJECT
    Q_DECLARE_PUBLIC(PollService)

public:
    PollServicePrivate(PollService *q);
    ~PollServicePrivate() {};

private Q_SLOTS:
    void poll();

private:
    PollService *q_ptr;
};

} // namespace

PollServicePrivate::PollServicePrivate(PollService *q):
    QObject(q),
    q_ptr(q)
{
}

void PollServicePrivate::poll()
{
}

PollService::PollService(QObject *parent):
    QObject(parent),
    d_ptr(new PollServicePrivate(this))
{
}

PollService::~PollService()
{
    delete d_ptr;
}

void PollService::Poll()
{
    Q_D(PollService);

    DEBUG() << "Got Poll";
    QMetaObject::invokeMethod(d, "poll", Qt::QueuedConnection);
}

#include "poll_service.moc"
