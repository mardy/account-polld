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
    void accountReady(Accounts::Account *account, const QString &appKey,
                      const QVariantMap &auth = QVariantMap());
    static QString accountServiceKey(Accounts::AccountService *as);

private:
    Accounts::Manager m_manager;
    AppManager *m_appManager;
    Applications m_apps;
    QHash<QString,Accounts::Application> m_accountApps;
    QHash<QString,AuthState> m_authStates;
    AccountManager *q_ptr;
};

} // namespace

AccountManagerPrivate::AccountManagerPrivate(AccountManager *q,
                                             AppManager *appManager):
    QObject(q),
    m_appManager(appManager),
    q_ptr(q)
{
}

QString AccountManagerPrivate::accountServiceKey(Accounts::AccountService *as)
{
    return QString("%1-%2").arg(as->account()->id()).arg(as->service().name());
}

void AccountManagerPrivate::loadApplications()
{
    m_accountApps.clear();

    m_apps = m_appManager->applications();
    for (auto i = m_apps.constBegin(); i != m_apps.constEnd(); i++) {
        m_accountApps.insert(i.key(), m_manager.application(i.value().appId));
    }
}

void AccountManagerPrivate::accountReady(Accounts::Account *account,
                                         const QString &appKey,
                                         const QVariantMap &auth)
{
    Q_Q(AccountManager);
    AccountData accountData;
    accountData.pluginId = appKey;
    accountData.accountId = account->id();
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

            QVariantMap authReply = reply.toMap();
            AuthState &authState = m_authStates[accountServiceKey(as)];
            if (authState.needNewToken && authReply == authState.lastAuthReply) {
                /* This account won't work, don't even check it */
                return;
            }

            authState.needNewToken = false;
            authState.lastAuthReply = authReply;
            accountReady(as->account(), appKey, authReply);
        });
        QObject::connect(authSession, &SignOn::AuthSession::error,
                         [this,as](const SignOn::Error &error) {
            as->deleteLater();
            DEBUG() << "authentication error:" << error.message();
        });

        AuthState &authState = m_authStates[key];

        QVariantMap sessionData = authData.parameters();
        sessionData["UiPolicy"] = SignOn::NoUserInteractionPolicy;
        if (authState.needNewToken) {
            sessionData["ForceTokenRefresh"] = true;
        }
        authSession->process(sessionData, authData.mechanism());
    } else {
        accountReady(as->account(), appKey);
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

    Accounts::AccountIdList accountIds = d->m_manager.accountListEnabled();
    for (Accounts::AccountId accountId: accountIds) {
        Accounts::Account *account = d->m_manager.account(accountId);
        if (Q_UNLIKELY(!account)) continue;

        Accounts::ServiceList services = account->enabledServices();

        /* check if we have some plugins registered for this service */
        for (auto i = d->m_accountApps.constBegin();
             i != d->m_accountApps.constEnd(); i++) {
            for (Accounts::Service &service: services) {
                if (!i.value().serviceUsage(service).isEmpty()) {
                    auto *as = new Accounts::AccountService(account, service);
                    d->activateAccount(as, i.key());
                }
            }
        }
    }
}

#include "account_manager.moc"
