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

#include "account_manager.h"

#include "app_manager.h"
#include "debug.h"

#include <Accounts/Account>
#include <Accounts/AccountService>
#include <Accounts/Application>
#include <Accounts/Manager>
#include <Accounts/Service>
#include <QMetaObject>
#include <SignOn/AuthSession>
#include <SignOn/Identity>
#include <SignOn/SessionData>

using namespace AccountPolld;

namespace AccountPolld {

class AccountManagerPrivate: public QObject
{
    Q_OBJECT
    Q_DECLARE_PUBLIC(AccountManager)

    struct AuthState {
        QVariantMap lastAuthReply;
        bool needNewToken;
    };

public:
    AccountManagerPrivate(AccountManager *q, AppManager *appManager);
    ~AccountManagerPrivate() {};

    void loadApplications();
    void activateAccount(Accounts::AccountService *as,
                         const QString &appKey);
    void accountReady(Accounts::AccountService *as, const QString &appKey,
                      const QVariantMap &auth = QVariantMap());
    static QString accountServiceKey(Accounts::AccountService *as);
    static QString accountServiceKey(uint accountId, const QString &serviceId);

    void markAuthFailure(const AccountData &data);
    QVariantMap formatAuthReply(const Accounts::AuthData &authData,
                                const QVariantMap &reply) const;

public Q_SLOTS:
    void operationFinished();

private:
    Accounts::Manager m_manager;
    AppManager *m_appManager;
    Applications m_apps;
    QHash<QString,Accounts::Application> m_accountApps;
    QHash<QString,AuthState> m_authStates;
    int m_pendingOperations;
    AccountManager *q_ptr;
};

uint qHash(const AccountData &data) {
    return ::qHash(data.pluginId) + ::qHash(data.accountId) +
        ::qHash(data.serviceId);
}

} // namespace

AccountManagerPrivate::AccountManagerPrivate(AccountManager *q,
                                             AppManager *appManager):
    QObject(q),
    m_appManager(appManager),
    m_pendingOperations(0),
    q_ptr(q)
{
    qRegisterMetaType<AccountData>("AccountData");
}

QString AccountManagerPrivate::accountServiceKey(Accounts::AccountService *as)
{
    return accountServiceKey(as->account()->id(), as->service().name());
}

QString AccountManagerPrivate::accountServiceKey(uint accountId, const QString &serviceId)
{
    return QString("%1-%2").arg(accountId).arg(serviceId);
}

void AccountManagerPrivate::loadApplications()
{
    m_accountApps.clear();

    m_apps = m_appManager->applications();
    for (auto i = m_apps.constBegin(); i != m_apps.constEnd(); i++) {
        Accounts::Application app = m_manager.application(i.value().appId);
        if (app.isValid()) {
            m_accountApps.insert(i.key(), app);
        } else {
            DEBUG() << "Application not found:" << i.value().appId;
        }
    }
}

QVariantMap AccountManagerPrivate::formatAuthReply(const Accounts::AuthData &authData,
                                                   const QVariantMap &reply) const
{
    QVariantMap formattedReply(reply);

    QString mechanism = authData.mechanism();
    const QVariantMap &parameters = authData.parameters();
    if (mechanism == "HMAC-SHA1" || mechanism == "PLAINTEXT") {
        /* For OAuth 1.0, let's return also the Consumer key and secret along
         * with the reply. */
        formattedReply["ClientId"] = parameters.value("ConsumerKey");
        formattedReply["ClientSecret"] = parameters.value("ConsumerSecret");
    } else if (mechanism == "web_server" || mechanism == "user_agent") {
        formattedReply["ClientId"] = parameters.value("ClientId");
        formattedReply["ClientSecret"] = parameters.value("ClientId");
    }

    return formattedReply;
}

void AccountManagerPrivate::accountReady(Accounts::AccountService *as,
                                         const QString &appKey,
                                         const QVariantMap &auth)
{
    Q_Q(AccountManager);
    AccountData accountData;
    accountData.pluginId = appKey;
    accountData.accountId = as->account()->id();
    accountData.serviceId = as->service().name();
    accountData.auth = auth;
    QMetaObject::invokeMethod(q, "accountReady", Qt::QueuedConnection,
                              Q_ARG(AccountData, accountData));
}

void AccountManagerPrivate::activateAccount(Accounts::AccountService *as,
                                            const QString &appKey)
{
    const AppData &data = m_apps[appKey];
    if (data.needsAuthData) {
        Accounts::AuthData authData = as->authData();
        QString key = accountServiceKey(as);

        auto identity =
            SignOn::Identity::existingIdentity(authData.credentialsId(), as);
        auto authSession = identity->createSession(authData.method());
        QObject::connect(authSession, &SignOn::AuthSession::response,
                         [this,as,appKey](const SignOn::SessionData &reply) {
            as->deleteLater();

            QVariantMap authReply = formatAuthReply(as->authData(), reply.toMap());
            AuthState &authState = m_authStates[accountServiceKey(as)];
            if (authState.needNewToken && authReply == authState.lastAuthReply) {
                /* This account won't work, don't even check it */
                operationFinished();
                return;
            }

            authState.needNewToken = false;
            accountReady(as, appKey, authReply);
            operationFinished();
        });
        QObject::connect(authSession, &SignOn::AuthSession::error,
                         [this,as](const SignOn::Error &error) {
            as->deleteLater();
            operationFinished();
            DEBUG() << "authentication error:" << error.message();
        });

        AuthState &authState = m_authStates[key];

        QVariantMap sessionData = authData.parameters();
        sessionData["UiPolicy"] = SignOn::NoUserInteractionPolicy;
        if (authState.needNewToken) {
            sessionData["ForceTokenRefresh"] = true;
        }
        m_pendingOperations++;
        authSession->process(sessionData, authData.mechanism());
    } else {
        accountReady(as, appKey);
    }
}

void AccountManagerPrivate::markAuthFailure(const AccountData &data)
{
    QString key = accountServiceKey(data.accountId, data.serviceId);
    AuthState &authState = m_authStates[key];
    authState.lastAuthReply = data.auth;
    authState.needNewToken = true;
}

void AccountManagerPrivate::operationFinished()
{
    Q_Q(AccountManager);
    m_pendingOperations--;
    if (m_pendingOperations == 0) {
        /* since the accountReady signal is sent in a queued connection, this
         * signal must also be sent in that way, in order to be delivered after
         * all the accountReady signals. */
        QMetaObject::invokeMethod(q, "finished", Qt::QueuedConnection);
    }
}

AccountManager::AccountManager(AppManager *appManager, QObject *parent):
    QObject(parent),
    d_ptr(new AccountManagerPrivate(this, appManager))
{
}

AccountManager::~AccountManager()
{
    delete d_ptr;
}

void AccountManager::listAccounts()
{
    Q_D(AccountManager);

    d->loadApplications();

    d->m_pendingOperations++;

    Accounts::AccountIdList accountIds = d->m_manager.accountListEnabled();
    for (Accounts::AccountId accountId: accountIds) {
        Accounts::Account *account = d->m_manager.account(accountId);
        if (Q_UNLIKELY(!account)) continue;

        Accounts::ServiceList services = account->enabledServices();

        /* check if we have some plugins registered for this service */
        for (auto i = d->m_accountApps.constBegin();
             i != d->m_accountApps.constEnd(); i++) {
            for (Accounts::Service &service: services) {
                /* Check if the application can use this service */
                if (i.value().serviceUsage(service).isEmpty()) {
                    continue;
                }

                /* Check if the plugin manifest allows using this service */
                const AppData &appData = d->m_apps[i.key()];
                if (!appData.services.isEmpty() &&
                    !appData.services.contains(service.name())) {
                    DEBUG() << "Skipping service" << service.name() <<
                        "for plugin" << i.key();
                    continue;
                }

                auto *as = new Accounts::AccountService(account, service);
                d->activateAccount(as, i.key());
            }
        }
    }

    d->operationFinished();
}

void AccountManager::markAuthFailure(const AccountData &data)
{
    Q_D(AccountManager);
    d->markAuthFailure(data);
}

#include "account_manager.moc"
