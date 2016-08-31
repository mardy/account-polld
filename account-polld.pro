include(common-vars.pri)
include(common-project-config.pri)

TEMPLATE = subdirs
SUBDIRS = \
    account-polld \
    goplugins.pro

include(common-installs-config.pri)
