'''push client mock template

This creates the expected methods and properties of the
com.ubuntu.Postal service.
'''

# This program is free software; you can redistribute it and/or modify it under
# the terms of the GNU Lesser General Public License as published by the Free
# Software Foundation; either version 2.1 of the License.  See
# http://www.gnu.org/copyleft/lgpl.html for the full text of the license.

__author__ = 'Alberto Mardegan'
__email__ = 'alberto.mardegan@canonical.com'
__copyright__ = '(c) 2016 Canonical Ltd.'
__license__ = 'LGPL 2.1'

import dbus
import time

from dbusmock import MOCK_IFACE
import dbusmock

BUS_NAME = 'com.ubuntu.Postal'
MAIN_OBJ = '/com/ubuntu/Postal'
MAIN_SERVICE_IFACE = 'com.ubuntu.Postal'
MAIN_IFACE = MAIN_SERVICE_IFACE
SYSTEM_BUS = False


def load(mock, parameters):
    mock.AddMethods(MAIN_SERVICE_IFACE, [
        ('Post', 'ss', '', 'ret = None'),
    ])

@dbus.service.method(MOCK_IFACE, in_signature='s', out_signature='')
def RegisterApp(self, path):
    self.AddObject(path, MAIN_IFACE, {}, [
        ('Post', 'ss', '', 'ret = None'),
    ])
