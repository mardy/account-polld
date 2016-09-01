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

#ifndef AP_PLUGIN_H
#define AP_PLUGIN_H

#include <QObject>

class QJsonObject;

namespace AccountPolld {

class PluginPrivate;
class Plugin: public QObject
{
    Q_OBJECT

public:
    explicit Plugin(const QString &execLine, const QString &profile,
                    QObject *parent = 0);
    ~Plugin();

    void run();
    void poll(const QJsonObject &pollData);

Q_SIGNALS:
    void ready();
    void response(const QJsonObject &resp);
    void finished();

private:
    PluginPrivate *d_ptr;
    Q_DECLARE_PRIVATE(Plugin)
};

} // namespace

#endif // AP_PLUGIN_H
