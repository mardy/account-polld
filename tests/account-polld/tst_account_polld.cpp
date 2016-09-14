/*
 * Copyright (C) 2016 Canonical Ltd.
 *
 * Contact: Alberto Mardegan <alberto.mardegan@canonical.com>
 *
 * This file is part of account-polld
 *
 * This library is free software; you can redistribute it and/or
 * modify it under the terms of the GNU Lesser General Public License
 * version 2.1 as published by the Free Software Foundation.
 *
 * This library is distributed in the hope that it will be useful, but
 * WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
 * Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public
 * License along with this library; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA
 * 02110-1301 USA
 */

#include "fake_push_client.h"
#include "fake_signond.h"

#include <Accounts/Account>
#include <Accounts/Manager>
#include <Accounts/Service>
#include <QDBusPendingReply>
#include <QDebug>
#include <QDir>
#include <QJsonDocument>
#include <QJsonObject>
#include <QSignalSpy>
#include <QTemporaryDir>
#include <QTest>
#include <libqtdbusmock/DBusMock.h>
#include <libqtdbustest/QProcessDBusService.h>

using namespace QtDBusMock;

namespace QTest {
template<>
char *toString(const QSet<QString> &set)
{
    QByteArray ba = "QSet<QString>(";
    QStringList list = set.toList();
    ba += list.join(", ");
    ba += ")";
    return qstrdup(ba.data());
}
} // QTest namespace


#define ACCOUNT_POLLD_OBJECT_PATH \
    QStringLiteral("/com/ubuntu/AccountPolld")
#define ACCOUNT_POLLD_SERVICE_NAME \
    QStringLiteral("com.ubuntu.AccountPolld")
#define ACCOUNT_POLLD_INTERFACE ACCOUNT_POLLD_SERVICE_NAME

class AccountPolldTest: public QObject
{
    Q_OBJECT

public:
    AccountPolldTest();

private Q_SLOTS:
    void initTestCase();
    void init();
    void cleanup();

    void testNoAccounts();
    void testPluginInput_data();
    void testPluginInput();
    void testWithoutAuthentication_data();
    void testWithoutAuthentication();

Q_SIGNALS:
    void pollDone();

private:
    void writePluginsFile(const QString &contents);
    void writePluginConf(const QString &reply, double delay);
    void writePluginConf(const QJsonObject &reply, double delay);
    QList<QJsonObject> pluginInput() const;
    void setupEnvironment();
    void clearBaseDir();
    QDBusPendingReply<void> callPoll() {
        QDBusMessage msg =
            QDBusMessage::createMethodCall(ACCOUNT_POLLD_SERVICE_NAME,
                                           ACCOUNT_POLLD_OBJECT_PATH,
                                           ACCOUNT_POLLD_INTERFACE,
                                           "Poll");
        return m_conn.asyncCall(msg);
    }
    bool replyIsValid(const QDBusMessage &reply);

private:
    QTemporaryDir m_baseDir;
    QString m_pluginsFilePath;
    QString m_pluginConfFilePath;
    QString m_pluginDumpPath;
    QtDBusTest::DBusTestRunner m_dbus;
    QtDBusMock::DBusMock m_mock;
    QDBusConnection m_conn;
    QtDBusTest::DBusServicePtr m_accountPolld;
    FakePushClient m_pushClient;
    FakeSignond m_signond;
};

AccountPolldTest::AccountPolldTest():
    QObject(0),
    m_dbus((setupEnvironment(), DBUS_SESSION_CONFIG_FILE)),
    m_mock(m_dbus),
    m_conn(m_dbus.sessionConnection()),
    m_accountPolld(new QtDBusTest::QProcessDBusService(ACCOUNT_POLLD_SERVICE_NAME,
                                                       QDBusConnection::SessionBus,
                                                       ACCOUNT_POLLD_BINARY,
                                                       QStringList())),
    m_pushClient(&m_mock),
    m_signond(&m_mock)
{
    DBusMock::registerMetaTypes();

    m_conn.connect(ACCOUNT_POLLD_SERVICE_NAME,
                   ACCOUNT_POLLD_OBJECT_PATH,
                   ACCOUNT_POLLD_INTERFACE,
                   "Done",
                   this, SIGNAL(pollDone()));
}

void AccountPolldTest::writePluginsFile(const QString &contents)
{
    QFile file(m_pluginsFilePath);
    if (!file.open(QIODevice::WriteOnly | QIODevice::Text)) {
        qWarning() << "Could not write file" << m_pluginsFilePath;
        return;
    }

    file.write(contents.toUtf8());
}

void AccountPolldTest::writePluginConf(const QString &reply, double delay)
{
    writePluginConf(QJsonDocument::fromJson(reply.toUtf8()).object(),
                    delay);
}

void AccountPolldTest::writePluginConf(const QJsonObject &reply, double delay)
{
    QJsonObject contents;
    contents["reply"] = reply;
    contents["delay"] = delay;
    QFile file(m_pluginConfFilePath);
    if (!file.open(QIODevice::WriteOnly | QIODevice::Text)) {
        qWarning() << "Could not write file" << m_pluginsFilePath;
        return;
    }

    file.write(QJsonDocument(contents).toJson());
}

QList<QJsonObject> AccountPolldTest::pluginInput() const
{
    QList<QJsonObject> objects;

    QDir dir(m_pluginDumpPath);
    Q_FOREACH(const QString &filePath, dir.entryList({"*.dump"})) {
        QFile file(dir.filePath(filePath));
        if (!file.open(QIODevice::ReadOnly | QIODevice::Text)) {
            qWarning() << "Could not read file" << filePath;
            continue;
        }
        QJsonDocument doc = QJsonDocument::fromJson(file.readAll());
        objects.append(doc.object());
    }

    return objects;
}

void AccountPolldTest::setupEnvironment()
{
    QVERIFY(m_baseDir.isValid());
    QByteArray baseDirPath = m_baseDir.path().toUtf8();
    QDir baseDir(m_baseDir.path());

    qunsetenv("XDG_DATA_DIR");
    qunsetenv("XDG_DATA_HOME");
    qputenv("HOME", baseDirPath + "/home");

    m_pluginsFilePath =
        baseDir.filePath("home/.local/share/" PLUGIN_DATA_FILE);

    m_pluginConfFilePath =
        baseDir.filePath("home/.config/test_plugin.conf");
    m_pluginDumpPath =
        baseDir.filePath("home/.local/share/test_plugin");

    //qputenv("ACCOUNTS", baseDirPath + "/home/.config/libaccounts-glib");
    qputenv("AG_APPLICATIONS", TEST_DATA_DIR);
    qputenv("AG_SERVICES", TEST_DATA_DIR);
    qputenv("AG_SERVICE_TYPES", TEST_DATA_DIR);
    qputenv("AG_PROVIDERS", TEST_DATA_DIR);

    qputenv("SSO_USE_PEER_BUS", "0");

    qputenv("XDG_RUNTIME_DIR", baseDirPath + "/runtime-dir");

    qputenv("AP_LOGGING_LEVEL", "2");
    qputenv("AP_PLUGIN_TIMEOUT", "3");

    /* Make sure we accidentally don't talk to the developer's services running
     * in the session bus */
    qunsetenv("DBUS_SESSION_BUS_ADDRESS");
}

bool AccountPolldTest::replyIsValid(const QDBusMessage &msg)
{
    if (msg.type() == QDBusMessage::ErrorMessage) {
        qDebug() << "Error name:" << msg.errorName();
        qDebug() << "Error text:" << msg.errorMessage();
    }
    return msg.type() == QDBusMessage::ReplyMessage;
}

void AccountPolldTest::initTestCase()
{
    m_dbus.registerService(m_accountPolld);
    m_dbus.startServices();
}

void AccountPolldTest::init()
{
    QDir baseDir(m_baseDir.path());

    baseDir.mkpath("home");
    baseDir.mkpath("home/.config");
    baseDir.mkpath("home/.local/share/account-polld");
    baseDir.mkpath("home/.local/share/test_plugin");
    baseDir.mkpath("runtime-dir");
}

void AccountPolldTest::cleanup()
{
    if (QTest::currentTestFailed()) {
        m_baseDir.setAutoRemove(false);
        qDebug() << "Base dir:" << m_baseDir.path();
    } else {
        /* Delete all accounts */
        Accounts::Manager manager;
        Q_FOREACH(Accounts::AccountId id, manager.accountList()) {
            Accounts::Account *account = manager.account(id);
            QVERIFY(account);
            account->remove();
            account->syncAndBlock();
        }

        /* Delete plugin output files */
        QDir plugin_dumps(m_pluginDumpPath);
        Q_FOREACH(const QString &filePath, plugin_dumps.entryList({"*.dump"})) {
            plugin_dumps.remove(filePath);
        }
    }
}

void AccountPolldTest::testNoAccounts()
{
    QSignalSpy doneCalled(this, SIGNAL(pollDone()));
    auto call = callPoll();

    QVERIFY(doneCalled.wait());
    QCOMPARE(doneCalled.count(), 1);

    QVERIFY(call.isFinished());
    QVERIFY(replyIsValid(call.reply()));

    /* Check that there are no notifications */
    QList<MethodCall> calls =
        m_pushClient.mockedService().GetMethodCalls("Post");
    QCOMPARE(calls.count(), 0);
}

void AccountPolldTest::testPluginInput_data()
{
    QTest::addColumn<bool>("needsAuthentication");

    QTest::newRow("no authentication") <<
        false;
    QTest::newRow("with authentication") <<
        true;
}

void AccountPolldTest::testPluginInput()
{
    QFETCH(bool, needsAuthentication);

    /* prepare accounts */
    Accounts::Manager manager;
    Accounts::Service coolShare = manager.service("com.ubuntu.tests_coolshare");
    Accounts::Service coolMail = manager.service("coolmail");

    uint credentialsId = 45;

    Accounts::Account *account = manager.createAccount("cool");
    account->setDisplayName("account 0");
    account->setCredentialsId(credentialsId);
    account->setEnabled(true);
    account->selectService(coolMail);
    account->setEnabled(true);
    account->syncAndBlock();

    m_signond.addIdentity(credentialsId, QVariantMap());

    /* write plugins json file */
    writePluginsFile(QString(
        "{"
        "  \"mail_helper\": {\n"
        "    \"appId\": \"mailer\",\n"
        "    \"exec\": \"" PLUGIN_EXECUTABLE "\",\n"
        "    \"needsAuthData\": %1,\n"
        "    \"profile\": \"unconfined\"\n"
        "  }\n"
        "}").arg(needsAuthentication ? "true" : "false"));

    /* tell the poll plugin how to behave */
    writePluginConf("{ \"notifications\": [] }", 0.1);

    /* Start polling */
    QSignalSpy doneCalled(this, SIGNAL(pollDone()));
    auto call = callPoll();

    QVERIFY(doneCalled.wait());
    QCOMPARE(doneCalled.count(), 1);

    QVERIFY(call.isFinished());
    QVERIFY(replyIsValid(call.reply()));

    auto inputs = pluginInput();
    QCOMPARE(inputs.count(), 1);

    QVariantMap authData {
        { "UiPolicy", 2 },
        { "host", "coolmail.ex" },
        { "ClientId", "my-client-id" },
        { "ClientSecret", "my-client-secret" },
        { "ConsumerKey", "my-client-id" },
        { "ConsumerSecret", "my-client-secret" },
    };
    QVariantMap expectedAuthData = needsAuthentication ? authData : QVariantMap();

    QJsonObject input = inputs[0];
    QCOMPARE(input["accountId"].toInt(), int(account->id()));
    QCOMPARE(input["appId"].toString(), QString("mailer"));
    QCOMPARE(input["helperId"].toString(), QString("mail_helper"));
    QCOMPARE(input["auth"].toObject().toVariantMap(), expectedAuthData);
}

void AccountPolldTest::testWithoutAuthentication_data()
{
    QTest::addColumn<QString>("plugins");
    QTest::addColumn<QString>("pluginReply");
    QTest::addColumn<QStringList>("expectedAppIds");
    QTest::addColumn<QStringList>("expectedNotifications");

    QTest::newRow("no plugins") <<
        "{}" <<
        "{}" <<
        QStringList{} <<
        QStringList{};

    QTest::newRow("one plugin") <<
        "{"
        "  \"mail_helper\": {\n"
        "    \"appId\": \"mailer\",\n"
        "    \"exec\": \"" PLUGIN_EXECUTABLE "\",\n"
        "    \"needsAuthenticationData\": false,\n"
        "    \"profile\": \"unconfined\"\n"
        "  }\n"
        "}" <<
        "{"
        "  \"notifications\": [\n"
        "    {\n"
        "      \"message\": \"hello\"\n"
        "    },\n"
        "    {\n"
        "      \"message\": \"second\"\n"
        "    }\n"
        "  ]\n"
        "}" <<
        QStringList{ "mailer" } <<
        QStringList{ "{\"message\":\"hello\"}", "{\"message\":\"second\"}" };
}

void AccountPolldTest::testWithoutAuthentication()
{
    QFETCH(QString, plugins);
    QFETCH(QString, pluginReply);
    QFETCH(QStringList, expectedAppIds);
    QFETCH(QStringList, expectedNotifications);

    /* prepare accounts */
    Accounts::Manager manager;
    Accounts::Service coolShare = manager.service("com.ubuntu.tests_coolshare");
    Accounts::Service coolMail = manager.service("coolmail");

    Accounts::Account *account = manager.createAccount("cool");
    account->setDisplayName("disabled");
    account->setEnabled(false);
    account->syncAndBlock();

    account = manager.createAccount("cool");
    account->setDisplayName("all enabled");
    account->setEnabled(true);
    account->selectService(coolShare);
    account->setEnabled(true);
    account->selectService(coolMail);
    account->setEnabled(true);
    account->syncAndBlock();

    /* write plugins json file */
    writePluginsFile(plugins);

    /* tell the poll plugin how to behave */
    writePluginConf(pluginReply, 0.1);

    m_pushClient.mockedService().ClearCalls();

    /* Start polling */
    QSignalSpy doneCalled(this, SIGNAL(pollDone()));
    auto call = callPoll();

    QVERIFY(doneCalled.wait());
    QCOMPARE(doneCalled.count(), 1);

    QVERIFY(call.isFinished());
    QVERIFY(replyIsValid(call.reply()));

    /* Check that there are the expected notifications */
    QTRY_COMPARE(m_pushClient.mockedService().GetMethodCalls("Post").value().count(),
                 expectedNotifications.count());
    QList<MethodCall> calls =
        m_pushClient.mockedService().GetMethodCalls("Post");
    QStringList appIds;
    QStringList notifications;
    for (const auto &call: calls) {
        const QVariantList &args = call.args();
        appIds.append(args[0].toString());
        notifications.append(args[1].toString());
    }
    QCOMPARE(appIds.toSet(), expectedAppIds.toSet());
    QCOMPARE(notifications.toSet(), expectedNotifications.toSet());
}

QTEST_GUILESS_MAIN(AccountPolldTest);

#include "tst_account_polld.moc"
