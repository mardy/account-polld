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
    DEBUG_ENABLED \
    PLUGIN_DATA_FILE=\\\"$${PLUGIN_DATA_FILE}\\\"

SOURCES += \
    account_manager.cpp \
    app_manager.cpp \
    debug.cpp \
    main.cpp \
    plugin.cpp \
    poll_service.cpp \
    push_client.cpp

HEADERS += \
    account_manager.h \
    app_manager.h \
    debug.h \
    plugin.h \
    poll_service.h \
    push_client.h

include($${TOP_SRC_DIR}/common-installs-config.pri)
