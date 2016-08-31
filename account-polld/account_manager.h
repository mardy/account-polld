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

#ifndef AP_ACCOUNT_MANAGER_H
#define AP_ACCOUNT_MANAGER_H

#include <QObject>
#include <QVariantMap>

namespace AccountPolld {

struct AccountData {
    QString pluginId;
    uint accountId;
    QVariantMap auth;
};

class AppManager;

class AccountManagerPrivate;
class AccountManager: public QObject
{
    Q_OBJECT

public:
    explicit AccountManager(AppManager *appManager, QObject *parent = 0);
    ~AccountManager();

    /* Scan for accounts; for each valid account, the accountReady() signal
     * will be emitted */
    void listAccounts();

    /* Call when the authentication data for an account is refused by the
     * server because of token expiration */
    void markAuthFailure(const AccountData &data);

Q_SIGNALS:
    void accountReady(const AccountData &data);

private:
    AccountManagerPrivate *d_ptr;
    Q_DECLARE_PRIVATE(AccountManager)
};

} // namespace

#endif // AP_ACCOUNT_MANAGER_H
