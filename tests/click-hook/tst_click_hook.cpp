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

#include <QDebug>
#include <QDir>
#include <QFile>
#include <QJsonDocument>
#include <QJsonObject>
#include <QProcess>
#include <QSignalSpy>
#include <QTemporaryDir>
#include <QTest>
#include <QVariantMap>

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

template<>
char *toString(const QVariantMap &p)
{
    QJsonDocument doc(QJsonObject::fromVariantMap(p));
    QByteArray ba = doc.toJson(QJsonDocument::Compact);
    return qstrdup(ba.data());
}

} // QTest namespace

struct HookFile {
    QString package;
    QString fileName;
    QString contents;
};

Q_DECLARE_METATYPE(HookFile)

class ClickHookTest: public QObject
{
    Q_OBJECT

public:
    ClickHookTest();

private Q_SLOTS:
    void initTestCase();
    void cleanup();

    void testValidHooks_data();
    void testValidHooks();

private:
    void createDirs();
    bool runHookProcess();
    void writeSystemHookFile(const QString &name,
                             const QString &contents);
    void writePackageFile(const QString &name,
                          const QString &contents = QString());
    QVariantMap parseManifest() const;
    QString stripVersion(const QString &appId) const;

private:
    QTemporaryDir m_baseDir;
    QDir m_testDir;
    QDir m_localHooksDir;
    QDir m_systemHooksDir;
    QDir m_manifestDir;
    QDir m_packageDir;
};

ClickHookTest::ClickHookTest():
    QObject(0),
    m_testDir(m_baseDir.path()),
    m_localHooksDir(m_baseDir.path() + "/xdg_data_home/account-polld/plugins"),
    m_systemHooksDir(m_baseDir.path() + "/system-hooks/account-polld/plugins"),
    m_manifestDir(m_baseDir.path() + "/xdg_data_home/account-polld"),
    m_packageDir(m_baseDir.path() + "/package")
{
}

void ClickHookTest::cleanup()
{
    if (QTest::currentTestFailed()) {
        m_baseDir.setAutoRemove(false);
        qDebug() << "Base dir:" << m_baseDir.path();
    } else {
        QDir dir(m_baseDir.path());
        dir.removeRecursively();

        createDirs();
    }
}

void ClickHookTest::createDirs()
{
    m_localHooksDir.mkpath(".");
    m_systemHooksDir.mkpath(".");
    m_manifestDir.mkpath(".");
    m_packageDir.mkpath(".");
}

bool ClickHookTest::runHookProcess()
{
    QProcess process;
    process.setProcessChannelMode(QProcess::ForwardedChannels);
    process.start(HOOK_PROCESS);
    if (!process.waitForFinished()) return false;

    return process.exitCode() == EXIT_SUCCESS;
}

QString ClickHookTest::stripVersion(const QString &appId) const
{
    QStringList parts = appId.split('_').mid(0, 2);
    return parts.join('_');
}

void ClickHookTest::writeSystemHookFile(const QString &name,
                                        const QString &contents)
{
    QFile file(m_systemHooksDir.filePath(name));
    if (!file.open(QIODevice::WriteOnly | QIODevice::Text)) {
        qWarning() << "Could not write file" << name;
        return;
    }

    file.write(contents.toUtf8());
}

void ClickHookTest::writePackageFile(const QString &name,
                                     const QString &contents)
{
    QFileInfo fileInfo(name);
    QString profile = fileInfo.path();

    m_packageDir.mkpath(profile);
    QFile file(m_packageDir.filePath(name));
    if (!file.open(QIODevice::WriteOnly | QIODevice::Text)) {
        qWarning() << "Could not write file" << name;
        return;
    }

    file.write(contents.toUtf8());

    QString pluginId = stripVersion(profile);
    QFile::link(file.fileName(), m_localHooksDir.filePath(pluginId + ".json"));
}

QVariantMap ClickHookTest::parseManifest() const
{
    QVariantMap map;
    QFile file(m_manifestDir.filePath("plugins_data.json"));
    if (!file.open(QIODevice::ReadOnly | QIODevice::Text)) {
        qWarning() << "Could not open manifest file" << file.fileName();
        return map;
    }

    return QJsonDocument::fromJson(file.readAll()).object().toVariantMap();
}

void ClickHookTest::initTestCase()
{
    qputenv("XDG_DATA_HOME", m_testDir.path().toUtf8() + "/xdg_data_home");
    qputenv("XDG_DATA_DIRS", m_testDir.path().toUtf8() + "/system-hooks");

    createDirs();
}

void ClickHookTest::testValidHooks_data()
{
    QTest::addColumn<QVector<HookFile>>("hookFiles");
    QTest::addColumn<QVariantMap>("expectedManifest");

    QTest::newRow("no files") <<
        QVector<HookFile>() <<
        QVariantMap();

    QTest::newRow("simplest system file") <<
        QVector<HookFile> {
            {
                "", "system-app.json",
                "{\n"
                "  \"exec\": \"/usr/bin/helper\",\n"
                "  \"app_id\": \"my-system-app\"\n"
                "}"
            },
        } <<
        QVariantMap {
            { "system-app", QVariantMap {
                    { "appId", "my-system-app" },
                    { "exec", "/usr/bin/helper" },
                    { "needsAuthData", false },
                },
            },
        };

    QTest::newRow("full system file") <<
        QVector<HookFile> {
            {
                "", "system-app.json",
                "{\n"
                "  \"exec\": \"/usr/bin/helper\",\n"
                "  \"app_id\": \"my-system-app\",\n"
                "  \"needs_authentication_data\": true,\n"
                "  \"service_ids\": [ \"one\", \"two\" ],\n"
                "  \"interval\": 20\n"
                "}"
            },
        } <<
        QVariantMap {
            { "system-app", QVariantMap {
                    { "appId", "my-system-app" },
                    { "exec", "/usr/bin/helper" },
                    { "needsAuthData", true },
                    { "services", QStringList { "one", "two" } },
                    { "interval", 20 },
                },
            },
        };

    QTest::newRow("package file") <<
        QVector<HookFile> {
            {
                "package_helper_0.3", "polld-plugin.json",
                "{\n"
                "  \"exec\": \"/usr/bin/helper\",\n"
                "  \"app_id\": \"package_myapp\"\n"
                "}"
            },
        } <<
        QVariantMap {
            { "package_helper", QVariantMap {
                    { "appId", "package_myapp" },
                    { "exec", "/usr/bin/helper" },
                    { "needsAuthData", false },
                },
            },
        };

    QTest::newRow("package file + invalid") <<
        QVector<HookFile> {
            {
                "package_helper_0.3", "polld-plugin.json",
                "{\n"
                "  \"exec\": \"/usr/bin/helper\",\n"
                "  \"app_id\": \"package_myapp\"\n"
                "}"
            },
            {
                "other_helper_0.1", "polld-plugin.json",
                "{\n"
                "  \"exec\": \"/usr/bin/malicious\",\n"
                "  \"app_id\": \"package_myapp\"\n"
                "}"
            },
        } <<
        QVariantMap {
            { "package_helper", QVariantMap {
                    { "appId", "package_myapp" },
                    { "exec", "/usr/bin/helper" },
                    { "needsAuthData", false },
                },
            },
        };

    QTest::newRow("no app IDs") <<
        QVector<HookFile> {
            {
                "package_helper_0.3", "polld-plugin.json",
                "{\n"
                "  \"exec\": \"/usr/bin/helper\"\n"
                "}"
            },
        } <<
        QVariantMap {
            { "package_helper", QVariantMap {
                    { "appId", "package_helper" },
                    { "exec", "/usr/bin/helper" },
                    { "needsAuthData", false },
                },
            },
        };

}

void ClickHookTest::testValidHooks()
{
    QFETCH(QVector<HookFile>, hookFiles);
    QFETCH(QVariantMap, expectedManifest);

    for (auto &hook: hookFiles) {
        if (hook.package.isEmpty()) {
            writeSystemHookFile(hook.fileName, hook.contents);
        } else {
            writePackageFile(hook.package + "/" + hook.fileName, hook.contents);
        }
    }

    QVERIFY(runHookProcess());

    QCOMPARE(parseManifest(), expectedManifest);
}

QTEST_GUILESS_MAIN(ClickHookTest);

#include "tst_click_hook.moc"
