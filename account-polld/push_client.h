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

#ifndef AP_PUSH_CLIENT_H
#define AP_PUSH_CLIENT_H

#include <QObject>

class QJsonObject;

namespace AccountPolld {

class PushClientPrivate;
class PushClient: public QObject
{
    Q_OBJECT

public:
    explicit PushClient(QObject *parent = 0);
    ~PushClient();

    void post(const QString &appId, const QJsonObject &message);

private:
    PushClientPrivate *d_ptr;
    Q_DECLARE_PRIVATE(PushClient)
};

} // namespace

#endif // AP_PUSH_CLIENT_H
