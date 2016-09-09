include(../../common-project-config.pri)

TARGET = tst_click_hook

CONFIG += \
    debug

QT += \
    core \
    testlib

DEFINES += \
    DEBUG_ENABLED \
    HOOK_PROCESS=\\\"$${TOP_SRC_DIR}/click-hook/click-hook\\\"

SOURCES += \
    tst_click_hook.cpp

check.commands = "./$${TARGET}"
check.depends = $${TARGET}
QMAKE_EXTRA_TARGETS += check
