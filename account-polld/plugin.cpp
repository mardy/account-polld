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

#include "plugin.h"

#include "debug.h"

#include <QByteArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonParseError>
#include <QProcess>
#include <QTimer>
#include <signal.h>
#include <sys/types.h>

using namespace AccountPolld;

namespace AccountPolld {

class PluginPrivate: public QProcess
{
    Q_OBJECT
    Q_DECLARE_PUBLIC(Plugin)

public:
    PluginPrivate(Plugin *q, const QString &execLine, const QString &profile);
    ~PluginPrivate() {};

public Q_SLOTS:
    void onReadyRead();
    void killPlugin();

private:
    QString m_execLine;
    QString m_profile;
    QTimer m_timer;
    QByteArray m_inputBuffer;
    bool m_sigtermSent;
    Plugin *q_ptr;
};

} // namespace

PluginPrivate::PluginPrivate(Plugin *q,
                             const QString &execLine,
                             const QString &profile):
    QProcess(q),
    m_execLine(execLine),
    m_profile(profile),
    m_sigtermSent(false),
    q_ptr(q)
{
    int killTime = 10;
    QProcessEnvironment environment = QProcessEnvironment::systemEnvironment();
    if (environment.contains("AP_PLUGIN_TIMEOUT")) {
        killTime = environment.value("AP_PLUGIN_TIMEOUT").toInt();
    }
    m_timer.setInterval(killTime * 1000);
    m_timer.setSingleShot(true);

    setProcessChannelMode(QProcess::ForwardedErrorChannel);
    QObject::connect(this, SIGNAL(started()), &m_timer, SLOT(start()));
    QObject::connect(this, SIGNAL(started()), q, SIGNAL(ready()));
    QObject::connect(this, SIGNAL(finished(int,QProcess::ExitStatus)),
                     q, SIGNAL(finished()));
    QObject::connect(this, SIGNAL(readyReadStandardOutput()),
                     this, SLOT(onReadyRead()));
    QObject::connect(&m_timer, SIGNAL(timeout()), this, SLOT(killPlugin()));
}

void PluginPrivate::onReadyRead()
{
    Q_Q(Plugin);

    m_inputBuffer.append(readAllStandardOutput());
    QJsonParseError error;
    auto doc = QJsonDocument::fromJson(m_inputBuffer, &error);
    if (error.error == QJsonParseError::NoError) {
        m_inputBuffer.clear();
        Q_EMIT q->response(doc.object());
    }

    /* otherwise continue reasing, the object is probably uncomplete */
}

void PluginPrivate::killPlugin()
{
    pid_t pid = processId();
    DEBUG() << "killing plugin" << pid;
    if (!m_sigtermSent) {
        ::kill(pid, SIGTERM);
        m_sigtermSent = true;
        m_timer.setInterval(1 * 1000);
        m_timer.start();
    } else {
        ::kill(pid, SIGKILL);
    }
}

Plugin::Plugin(const QString &execLine, const QString &profile,
               QObject *parent):
    QObject(parent),
    d_ptr(new PluginPrivate(this, execLine, profile))
{
}

Plugin::~Plugin()
{
    delete d_ptr;
}

void Plugin::run()
{
    Q_D(Plugin);

    QString command;

    if (d->m_profile != "unconfined") {
        command = QString("aa-exec-click -p %1 -- ").arg(d->m_profile);
    }

    command.append(d->m_execLine);

    DEBUG() << "Starting" << command;
    d->start(command);
}

void Plugin::poll(const QJsonObject &pollData)
{
    Q_D(Plugin);

    DEBUG() << "Plugin input:" << pollData;
    d->write(QJsonDocument(pollData).toJson(QJsonDocument::Compact));
    d->write("\n");
}

#include "plugin.moc"
