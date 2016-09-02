include(common-vars.pri)
include(common-project-config.pri)

TEMPLATE = subdirs
SUBDIRS = \
    account-polld \
    click-hook

include(common-installs-config.pri)
