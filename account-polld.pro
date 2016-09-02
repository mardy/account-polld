include(common-vars.pri)
include(common-project-config.pri)

TEMPLATE = subdirs
SUBDIRS = \
    account-polld \
    click-hook \
    tests
CONFIG += ordered

include(common-installs-config.pri)
