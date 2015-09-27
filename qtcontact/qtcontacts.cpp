/*
 Copyright 2014 Canonical Ltd.

 This program is free software: you can redistribute it and/or modify it
 under the terms of the GNU General Public License version 3, as published
 by the Free Software Foundation.

 This program is distributed in the hope that it will be useful, but
 WITHOUT ANY WARRANTY; without even the implied warranties of
 MERCHANTABILITY, SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR
 PURPOSE.  See the GNU General Public License for more details.

 You should have received a copy of the GNU General Public License along
 with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

#include <QContactManager>
#include <QContactFilter>
#include <QContactEmailAddress>
#include <QContactDetailFilter>
#include <QContactManager>
#include <QContactAvatar>
#include <QCoreApplication>
#include <QScopedPointer>
#include <QTimer>
#include <thread>

#include "qtcontacts.h"
#include "qtcontacts.hpp"
#include "qtcontacts.moc"

#ifdef __cplusplus
extern "C" {
#include "_cgo_export.h"
}
#endif

#define trace(...) fprintf(stderr, __VA_ARGS__)

QTCONTACTS_USE_NAMESPACE

int mainloopStart() {
    static char empty[1] = {0};
    static char *argv[] = {empty, empty, empty};
    static int argc = 1;

    QCoreApplication mApp(argc, argv);
    return mApp.exec();
}

char* getAvatar(char *email) {
    QScopedPointer<Avatar> avatar(new Avatar());
    QString thumbnailPath = avatar->retrieveThumbnail(QString(email));

    QByteArray byteArray = thumbnailPath.toUtf8();
    char* cString = byteArray.data();

    return cString;
}

QString Avatar::retrieveThumbnail(const QString& email) {
    QString avatar;

    QContactManager manager ("galera");
    QContactDetailFilter filter(QContactEmailAddress::match(email));
    trace("before");
    QList<QContact> contacts = manager.contacts(filter);
    trace("after");
    // TODO: contacts() takes a long time to finish, improve this using the information from https://projects.kde.org/projects/qt5/qtpim/repository/revisions/master/entry/src/contacts/qcontactfetchhint.cpp and https://projects.kde.org/projects/qt5/qtpim/repository/revisions/master/entry/src/contacts/qcontactmanager.cpp
    // Does it help if we store the manager we use?
    if(contacts.size() > 0) {
        avatar = contacts[0].detail<QContactAvatar>().imageUrl().path();
    }

    return avatar;
}
