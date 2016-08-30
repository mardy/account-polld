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

#include <QCoreApplication>
#include <QDBusConnection>
#include <QProcessEnvironment>
#include <QSettings>

#include "debug.h"
#include "poll_service.h"


int main(int argc, char **argv)
{
    QCoreApplication app(argc, argv);

    QSettings settings("account-polld");

    /* read environment variables */
    QProcessEnvironment environment = QProcessEnvironment::systemEnvironment();
    if (environment.contains(QLatin1String("AP_LOGGING_LEVEL"))) {
        bool isOk;
        int value = environment.value(
            QLatin1String("AP_LOGGING_LEVEL")).toInt(&isOk);
        if (isOk)
            setLoggingLevel(value);
    } else {
        setLoggingLevel(settings.value("LoggingLevel", 1).toInt());
    }

    QDBusConnection connection = QDBusConnection::sessionBus();

    auto service = new AccountPolld::PollService();
    connection.registerObject(ACCOUNT_POLLD_OBJECT_PATH, service);
    connection.registerService(ACCOUNT_POLLD_SERVICE_NAME);


    int ret = app.exec();

    connection.unregisterService(ACCOUNT_POLLD_SERVICE_NAME);
    connection.unregisterObject(ACCOUNT_POLLD_OBJECT_PATH);
    delete service;

    return ret;
}

