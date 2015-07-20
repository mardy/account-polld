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
#include <stdio.h>
#include <string.h>

#include <glib.h>
#include <libaccounts-glib/accounts-glib.h>
#include <libsignon-glib/signon-glib.h>

#include "account-watcher.h"

/* #define DEBUG */
#ifdef DEBUG
#  define trace(...) fprintf(stderr, __VA_ARGS__)
#else
#  define trace(...)
#endif

struct _AccountWatcher {
    AgManager *manager;
    /* A hash table of the enabled accounts we know of.
     * Keys are account ID integers, and AccountInfo structs as values.
     */
    GHashTable *services;

    gulong enabled_event_signal_id;
    gulong account_deleted_signal_id;

    AccountEnabledCallback callback;
    void *user_data;
};

typedef struct _AccountInfo AccountInfo;
struct _AccountInfo {
    AccountWatcher *watcher;
    /* Manage signin session for account */
    AgAccountService *account_service;
    SignonAuthSession *session;
    GVariant *auth_params;
    GVariant *session_data;

    gulong enabled_signal_id;
    AgAccountId account_id;
    gboolean enabled; /* the last known state of the account */
};

static void account_info_clear_login(AccountInfo *info) {
    if (info->session_data) {
        g_variant_unref(info->session_data);
        info->session_data = NULL;
    }
    if (info->auth_params) {
        g_variant_unref(info->auth_params);
        info->auth_params = NULL;
    }
    if (info->session) {
        signon_auth_session_cancel(info->session);
        g_object_unref(info->session);
        info->session = NULL;
    }
}

static void account_info_free(AccountInfo *info) {
    account_info_clear_login(info);
    if (info->enabled_signal_id != 0) {
        g_signal_handler_disconnect(
            info->account_service, info->enabled_signal_id);
    }
    info->enabled_signal_id = 0;
    if (info->account_service) {
        g_object_unref(info->account_service);
        info->account_service = NULL;
    }
    g_free(info);
}

static void account_info_notify(AccountInfo *info, GError *error) {
    AgService *service = ag_account_service_get_service(info->account_service);

    const char *service_name = ag_service_get_name(service);
    char *client_id = NULL;
    char *client_secret = NULL;
    char *access_token = NULL;
    char *token_secret = NULL;
    /* char *type = NULL; */ /* TODO: type, other names when password authentication? */

    if (info->auth_params != NULL) {
        /* Look up OAuth 2 parameters */
        g_variant_lookup(info->auth_params, "ClientId", "&s", &client_id);
        g_variant_lookup(info->auth_params, "ClientSecret", "&s", &client_secret);
        /* Fall back to OAuth 1 names if no OAuth 2 parameters could be found */
        if (client_id != NULL && client_secret != NULL && strcmp(client_id, "") != 0 && strcmp(client_secret, "") != 0) {
            /* type = "oauth2" */
        } else {
            g_variant_lookup(info->auth_params, "ConsumerKey", "&s", &client_id);
            g_variant_lookup(info->auth_params, "ConsumerSecret", "&s", &client_secret);
            /* Fall back to password authentication if no OAuth 1 parameters could be found */
            if (client_id != NULL && client_secret != NULL && strcmp(client_id, "") != 0 && strcmp(client_secret, "") != 0) {
                /* type = "oauth1" */
            } else {
                g_variant_lookup(info->auth_params, "UserName", "&s", &client_id);
                g_variant_lookup(info->auth_params, "Secret", "&s", &client_secret);
                if (client_id != NULL && client_secret != NULL && strcmp(client_id, "") != 0 && strcmp(client_secret, "") != 0) {
                    /* type = "password" */
                }
            }
        }
    }
    if (info->session_data != NULL) { /* TODO: && type != "password" */
        g_variant_lookup(info->session_data, "AccessToken", "&s", &access_token);
        g_variant_lookup(info->session_data, "TokenSecret", "&s", &token_secret);
    }

    info->watcher->callback(info->watcher,
                            info->account_id,
                            service_name,
                            error,
                            info->enabled,
                            client_id,
                            client_secret,
                            access_token,
                            token_secret,
                            info->watcher->user_data);
}

static void account_info_login_cb(GObject *source, GAsyncResult *result, void *user_data) {
    SignonAuthSession *session = (SignonAuthSession *)source;
    AccountInfo *info = (AccountInfo *)user_data;

    trace("Authentication for account %u complete\n", info->account_id);

    GError *error = NULL;
    info->session_data = signon_auth_session_process_finish(session, result, &error);
    account_info_notify(info, error);

    if (error != NULL) {
        trace("Authentication failed: %s\n", error->message);
        g_error_free(error);
    }
}

static void account_info_login(AccountInfo *info) {
    account_info_clear_login(info);

    AgAuthData *auth_data = ag_account_service_get_auth_data(info->account_service);
    GError *error = NULL;
    trace("Starting authentication session for account %u\n", info->account_id);
    info->session = signon_auth_session_new(
        ag_auth_data_get_credentials_id(auth_data),
        ag_auth_data_get_method(auth_data), &error);
    if (error != NULL) {
        trace("Could not set up auth session: %s\n", error->message);
        account_info_notify(info, error);
        g_error_free(error);
        g_object_unref(auth_data);
        return;
    }

    info->auth_params = g_variant_ref_sink(
        ag_auth_data_get_login_parameters(
            auth_data, NULL));

    signon_auth_session_process_async(
        info->session,
        info->auth_params,
        ag_auth_data_get_mechanism(auth_data),
        NULL, /* cancellable */
        account_info_login_cb, info);
    ag_auth_data_unref(auth_data);
}

static void account_info_enabled_cb(
    AgAccountService *account_service, gboolean enabled, AccountInfo *info) {
    trace("account_info_enabled_cb for %u, enabled=%d\n", info->account_id, enabled);
    if (info->enabled == enabled) {
        /* no change */
        return;
    }
    info->enabled = enabled;

    if (enabled) {
        account_info_login(info);
    } else {
        account_info_clear_login(info);
        // Send notification that account has been disabled */
        account_info_notify(info, NULL);
    }
}

static AccountInfo *account_info_new(AccountWatcher *watcher, AgAccountService *account_service) {
    AccountInfo *info = g_new0(AccountInfo, 1);
    info->watcher = watcher;
    info->account_service = g_object_ref(account_service);

    AgAccount *account = ag_account_service_get_account(account_service);
    g_object_get(account, "id", &info->account_id, NULL);

    info->enabled_signal_id = g_signal_connect(
        account_service, "enabled",
        G_CALLBACK(account_info_enabled_cb), info);
    // Set initial state
    account_info_enabled_cb(account_service, ag_account_service_get_enabled(account_service), info);

    return info;
}

static void account_watcher_enabled_event_cb(
    AgManager *manager, AgAccountId account_id, AccountWatcher *watcher) {
    trace("enabled-event for %u\n", account_id);
    if (g_hash_table_contains(watcher->services, GUINT_TO_POINTER(account_id))) {
        /* We are already tracking this account */
        return;
    }
    AgAccount *account = ag_manager_get_account(manager, account_id);
    if (account == NULL) {
        /* There was a problem looking up the account */
        return;
    }
    /* Since our AgManager is restricted to a particular service type,
     * pick the first service for the account. */
    GList *services = ag_account_list_services(account);
    if (services != NULL) {
        AgService *service = services->data;
        AgAccountService *account_service = ag_account_service_new(
            account, service);
        AccountInfo *info = account_info_new(watcher, account_service);
        g_object_unref(account_service);
        g_hash_table_insert(watcher->services, GUINT_TO_POINTER(account_id), info);
    }
    ag_service_list_free(services);
    g_object_unref(account);
}

static void account_watcher_account_deleted_cb(
    AgManager *manager, AgAccountId account_id, AccountWatcher *watcher) {
    trace("account-deleted for %u\n", account_id);
    /* A disabled event should have been sent prior to this, so no
     * need to send any notification. */
    g_hash_table_remove(watcher->services, GUINT_TO_POINTER(account_id));
}

static gboolean account_watcher_setup(void *user_data) {
    AccountWatcher *watcher = (AccountWatcher *)user_data;

    /* Track changes to accounts */
    watcher->enabled_event_signal_id = g_signal_connect(
        watcher->manager, "enabled-event",
        G_CALLBACK(account_watcher_enabled_event_cb), watcher);
    watcher->account_deleted_signal_id = g_signal_connect(
        watcher->manager, "account-deleted",
        G_CALLBACK(account_watcher_account_deleted_cb), watcher);

    /* Now check initial state */
    GList *enabled_accounts = ag_manager_list(watcher->manager);
    GList *l;
    for (l = enabled_accounts; l != NULL; l = l->next) {
        AgAccountId account_id = GPOINTER_TO_UINT(l->data);
        account_watcher_enabled_event_cb(watcher->manager, account_id, watcher);
    }
    ag_manager_list_free(enabled_accounts);

    return G_SOURCE_REMOVE;
}

AccountWatcher *account_watcher_new(const char *service_type,
                                    AccountEnabledCallback callback,
                                    void *user_data) {
    AccountWatcher *watcher = g_new0(AccountWatcher, 1);

    watcher->manager = ag_manager_new_for_service_type(service_type);
    watcher->services = g_hash_table_new_full(
        g_direct_hash, g_direct_equal, NULL, (GDestroyNotify)account_info_free);
    watcher->callback = callback;
    watcher->user_data = user_data;

    /* Make sure main setup occurs within the mainloop thread */
    g_idle_add(account_watcher_setup, watcher);
    return watcher;
}

struct refresh_info {
    AccountWatcher *watcher;
    AgAccountId account_id;
};

static gboolean account_watcher_refresh_cb(void *user_data) {
    struct refresh_info *data = (struct refresh_info *)user_data;

    AccountInfo *info = g_hash_table_lookup(
        data->watcher->services, GUINT_TO_POINTER(data->account_id));
    if (info != NULL) {
        account_info_login(info);
    }

    return G_SOURCE_REMOVE;
}

void account_watcher_refresh(AccountWatcher *watcher, unsigned int account_id) {
    struct refresh_info *data = g_new(struct refresh_info, 1);
    data->watcher = watcher;
    data->account_id = account_id;
    g_idle_add_full(G_PRIORITY_DEFAULT_IDLE, account_watcher_refresh_cb,
                    data, g_free);
}
