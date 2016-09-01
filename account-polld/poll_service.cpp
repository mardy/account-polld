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
#include "account_manager.h"
#include "app_manager.h"
#include "poll_service.h"
#include "plugin.h"
#include "push_client.h"

#include <QDateTime>
#include <QDBusArgument>
#include <QDBusConnection>
#include <QJsonArray>
#include <QJsonObject>
#include <QVariantMap>

using namespace AccountPolld;

namespace AccountPolld {

class PollServicePrivate: public QObject
{
    Q_OBJECT
    Q_DECLARE_PUBLIC(PollService)

    struct PollData {
        QDateTime lastPolled;
    };

public:
    PollServicePrivate(PollService *q);
    ~PollServicePrivate() {};

    QJsonObject preparePluginInput(const AccountData &accountData,
                                   const AppData &appData);
    void handleResponse(const QJsonObject &response, const QString &appId,
                        const AccountData &accountData);

private Q_SLOTS:
    void poll();
    void onAccountReady(const AccountData &data);

private:
    AppManager m_appManager;
    AccountManager m_accountManager;
    PushClient m_pushClient;
    QHash<AccountData,PollData> m_polls;
    PollService *q_ptr;
};

} // namespace

PollServicePrivate::PollServicePrivate(PollService *q):
    QObject(q),
    m_accountManager(&m_appManager),
    q_ptr(q)
{
    QObject::connect(&m_accountManager,
                     SIGNAL(accountReady(const AccountData&)),
                     this,
                     SLOT(onAccountReady(const AccountData&)));
}

QJsonObject
PollServicePrivate::preparePluginInput(const AccountData &accountData,
                                       const AppData &appData)
{
    QJsonObject object;
    object["helperId"] = accountData.pluginId;
    object["appId"] = appData.appId;
    object["accountId"] = int(accountData.accountId);
    if (appData.needsAuthData) {
        object["auth"] = QJsonObject::fromVariantMap(accountData.auth);
    }
    return object;
}

void PollServicePrivate::handleResponse(const QJsonObject &response,
                                        const QString &appId,
                                        const AccountData &accountData)
{
    QJsonObject error = response["error"].toObject();
    if (error["code"].toString() == "ERR_INVALID_AUTH") {
        m_accountManager.markAuthFailure(accountData);
        return;
    }

    QJsonArray notifications = response["notifications"].toArray();
    for (const QJsonValue &v: notifications) {
        m_pushClient.post(appId, v.toObject());
    }
}

void PollServicePrivate::poll()
{
    m_accountManager.listAccounts();
}

void PollServicePrivate::onAccountReady(const AccountData &accountData)
{
    Applications apps = m_appManager.applications();
    const auto i = apps.find(accountData.pluginId);
    if (i == apps.end()) {
        qWarning() << "Got account for plugin, but no app linked:" << accountData.pluginId;
        return;
    }

    const AppData &appData = i.value();

    /* Check that we are not polling more often than what the application
     * wishes to */
    PollData &pollData = m_polls[accountData];
    QDateTime now = QDateTime::currentDateTime();
    if (pollData.lastPolled.isValid() &&
        pollData.lastPolled.secsTo(now) < appData.interval) {
        DEBUG() << "Skipping poll, interval not yet expired:" << accountData.pluginId;
        return;
    }
    pollData.lastPolled = now;

    QJsonObject pluginInput = preparePluginInput(accountData, appData);

    Plugin *plugin = new Plugin(appData.execLine, appData.profile, this);
    QObject::connect(plugin, SIGNAL(finished()), plugin, SLOT(deleteLater));
    QObject::connect(plugin, &Plugin::ready,
                     [plugin, pluginInput]() { plugin->poll(pluginInput); });
    QObject::connect(plugin, &Plugin::response,
                     [this, accountData, appData](const QJsonObject &resp) {
        handleResponse(resp, appData.appId, accountData);
    });

    plugin->run();
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
