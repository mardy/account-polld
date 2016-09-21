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
#include "app_manager.h"

#include <QFile>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QStandardPaths>

using namespace AccountPolld;

namespace AccountPolld {

class AppManagerPrivate: public QObject
{
    Q_OBJECT
    Q_DECLARE_PUBLIC(AppManager)

public:
    AppManagerPrivate(AppManager *q);
    ~AppManagerPrivate() {};

    Applications readPluginData() const;

private:
    QString m_dataFilePath;
    AppManager *q_ptr;
};

} // namespace

AppManagerPrivate::AppManagerPrivate(AppManager *q):
    QObject(q),
    q_ptr(q)
{
    const QString localShare =
        QStandardPaths::writableLocation(QStandardPaths::GenericDataLocation);
    m_dataFilePath = localShare + "/" PLUGIN_DATA_FILE;
}

Applications AppManagerPrivate::readPluginData() const
{
    Applications apps;

    QFile file(m_dataFilePath);
    if (!file.open(QIODevice::ReadOnly)) return apps;

    QJsonDocument doc = QJsonDocument::fromJson(file.readAll());
    file.close();

    QJsonObject mainObject = doc.object();
    for (auto i = mainObject.begin(); i != mainObject.end(); i++) {
        QJsonObject appObject = i.value().toObject();

        AppData data;
        data.profile = appObject.value("profile").toString();
        data.execLine = appObject.value("exec").toString();
        data.appId = appObject.value("appId").toString();
        QJsonArray services = appObject.value("services").toArray();
        for (const QJsonValue &v: services) {
            data.services.append(v.toString());
        }
        data.interval = appObject.value("interval").toInt();
        data.needsAuthData = appObject.value("needsAuthData").toBool();

        if (data.profile.isEmpty() ||
            data.execLine.isEmpty() ||
            data.appId.isEmpty()) {
            qWarning() << "Incomplete plugin data:" <<
                QJsonDocument(appObject).toJson(QJsonDocument::Compact);
            continue;
        }

        apps.insert(i.key(), data);
    }

    return apps;
}

AppManager::AppManager(QObject *parent):
    QObject(parent),
    d_ptr(new AppManagerPrivate(this))
{
}

AppManager::~AppManager()
{
    delete d_ptr;
}

Applications AppManager::applications() const
{
    Q_D(const AppManager);

    return d->readPluginData();
}

#include "app_manager.moc"
