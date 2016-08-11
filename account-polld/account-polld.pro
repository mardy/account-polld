include(../common-project-config.pri)
include($${TOP_SRC_DIR}/common-vars.pri)

TEMPLATE = app
TARGET = account-polld

CONFIG += \
    link_pkgconfig \
    no_keywords \
    qt

QT += \
    dbus

PKGCONFIG += \
    accounts-qt5 \
    libsignon-qt5

DEFINES += \
    DEBUG_ENABLED

SOURCES += \
    debug.cpp \
    main.cpp

HEADERS += \
    debug.h \

include($${TOP_SRC_DIR}/common-installs-config.pri)
