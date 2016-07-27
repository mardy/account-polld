/*
 Copyright 2014 Canonical Ltd.

 This program is free software: you can redistribute it and/or modify it
 under the terms of the GNU General Public License version 3, as published
 by the Free Software Foundation.

 This program is distributed in the hope that it will be useful, but
 WITHOUT ANY WARRANTY; without even the implied warranties of
 MERCHANTABILITY, SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR
 PURPOSE.  See the GNU General Public License for more details.

 You should have received a copy of the GNU General Public License along
 with this program.  If not, see <http://www.gnu.org/licenses/>.
*/
#ifndef ACCOUNT_WATCHER_H
#define ACCOUNT_WATCHER_H

#include <glib.h>

typedef struct _AccountWatcher AccountWatcher;

typedef void (*AccountEnabledCallback)(AccountWatcher *watcher,
                                       unsigned int account_id,
                                       const char *service_type,
                                       const char *service_name,
                                       GError *error,
                                       int enabled,
                                       const char *client_id,
                                       const char *client_secret,
                                       const char *access_token,
                                       const char *token_secret,
                                       void *user_data);

AccountWatcher *account_watcher_new(AccountEnabledCallback callback,
                                    void *user_data);
void account_watcher_add_service(AccountWatcher *watcher,
                                 char *serviceId);
void account_watcher_run(AccountWatcher *watcher);

void account_watcher_refresh(AccountWatcher *watcher,
                             unsigned int account_id,
                             const char *service_name);

#endif
