include(../common-project-config.pri)

TEMPLATE = aux

QMAKE_SUBSTITUTES += \
    account-polld.hook.in

hook_helper.files = \
    click-hook
hook_helper.path = $${INSTALL_PREFIX}/lib/account-polld
INSTALLS += hook_helper

hooks.files = \
    account-polld.hook
hooks.path = $${INSTALL_PREFIX}/share/click/hooks
INSTALLS += hooks

include($${TOP_SRC_DIR}/common-installs-config.pri)
