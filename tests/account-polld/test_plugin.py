#!/usr/bin/python3
# -*- python -*-

import json
import os
import sys
import time

import xdg.BaseDirectory

class Plugin:
    def __init__(self):
        xdg_config_home = xdg.BaseDirectory.xdg_config_home
        self.conf_file = os.path.join(xdg_config_home, 'test_plugin.conf')

    def run(self):
        # dump the input received
        query = json.loads(sys.stdin.readline())
        xdg_data_home = xdg.BaseDirectory.xdg_data_home
        with open(os.path.join(xdg_data_home, 'test_plugin', '%s.dump' % os.getpid()), 'w') as fd:
            json.dump(query, fd)

        self.conf = {}
        try:
            with open(self.conf_file, 'r') as fd:
                self.conf = json.load(fd)
        except Exception as e:
            print('Unable to parse JSON from %s - %s' % (self.conf_file, e), file=sys.stderr)
            return False

        delay = self.conf.get('delay', 0)
        if delay:
            time.sleep(delay)

        reply = self.conf['reply']
        json.dump(reply, sys.stdout)


if __name__ == "__main__":
    plugin = Plugin()
    ok = plugin.run()

    sys.exit(0 if ok else 1)
