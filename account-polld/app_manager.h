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

#ifndef AP_APP_MANAGER_H
#define AP_APP_MANAGER_H

#include <QObject>
#include <QStringList>
#include <QHash>

namespace AccountPolld {

struct AppData {
    QString profile; // apparmor label for the plugin process
    QString execLine;
    QString appId; // appId, for matching with OA
    QStringList services;
    int interval;
    bool needsAuthData;
};

typedef QHash<QString,AppData> Applications;

class AppManagerPrivate;

class AppManager: public QObject
{
    Q_OBJECT

public:
    explicit AppManager(QObject *parent = 0);
    ~AppManager();

    Applications applications() const;

private:
    AppManagerPrivate *d_ptr;
    Q_DECLARE_PRIVATE(AppManager)
};

} // namespace

#endif // AP_APP_MANAGER_H
