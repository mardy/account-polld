include(../../common-project-config.pri)

TEMPLATE = app
TARGET = tst_account_polld

CONFIG += \
    c++11 \
    debug \
    link_pkgconfig

QT += \
    core \
    dbus \
    testlib

PKGCONFIG += \
    accounts-qt5 \
    libqtdbusmock-1 \
    libqtdbustest-1

DEFINES += \
    ACCOUNT_POLLD_BINARY=\\\"$${TOP_BUILD_DIR}/account-polld/account-polld\\\" \
    PLUGIN_DATA_FILE=\\\"$${PLUGIN_DATA_FILE}\\\" \
    PLUGIN_EXECUTABLE=\\\"$${PWD}/plugin.py\\\" \
    PUSH_CLIENT_MOCK_TEMPLATE=\\\"$${PWD}/push_client.py\\\" \
    TEST_DATA_DIR=\\\"$${PWD}/data\\\"

SOURCES += \
    tst_account_polld.cpp

HEADERS += \
    fake_push_client.h

check.commands = "./$${TARGET}"
check.depends = $${TARGET}
