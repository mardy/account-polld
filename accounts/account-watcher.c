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
     * Keys are "<accountId>/<serviceName>", and AccountInfo structs as values.
     */
    GHashTable *services;

    /* List of supported services' IDs */
    GSList *supported_services;

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

    AgAccountId account_id;
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
    if (info->account_service) {
        g_object_unref(info->account_service);
        info->account_service = NULL;
    }
    g_free(info);
}

static void account_info_notify(AccountInfo *info, GError *error) {
    AgService *service = ag_account_service_get_service(info->account_service);
    const char *service_name = ag_service_get_name(service);
    const char *service_type = ag_service_get_service_type(service);
    char *client_id = NULL;
    char *client_secret = NULL;
    char *access_token = NULL;
    char *token_secret = NULL;

    if (info->auth_params != NULL) {
        /* Look up OAuth 2 parameters, falling back to OAuth 1 names */
        g_variant_lookup(info->auth_params, "ClientId", "&s", &client_id);
        g_variant_lookup(info->auth_params, "ClientSecret", "&s", &client_secret);
        if (client_id == NULL) {
            g_variant_lookup(info->auth_params, "ConsumerKey", "&s", &client_id);
        }
        if (client_secret == NULL) {
            g_variant_lookup(info->auth_params, "ConsumerSecret", "&s", &client_secret);
        }
    }
    if (info->session_data != NULL) {
        g_variant_lookup(info->session_data, "AccessToken", "&s", &access_token);
        g_variant_lookup(info->session_data, "TokenSecret", "&s", &token_secret);
    }

    info->watcher->callback(info->watcher,
                            info->account_id,
                            service_type,
                            service_name,
                            error,
                            TRUE,
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

    /* Tell libsignon-glib not to open a trust session as we have no UI */
    GVariantBuilder builder;
    g_variant_builder_init(&builder, G_VARIANT_TYPE_VARDICT);
    g_variant_builder_add(&builder, "{sv}",
        SIGNON_SESSION_DATA_UI_POLICY,
        g_variant_new_int32(SIGNON_POLICY_NO_USER_INTERACTION));

    info->auth_params = g_variant_ref_sink(
        ag_auth_data_get_login_parameters(
            auth_data,
            g_variant_builder_end(&builder)));

    signon_auth_session_process_async(
        info->session,
        info->auth_params,
        ag_auth_data_get_mechanism(auth_data),
        NULL, /* cancellable */
        account_info_login_cb, info);
    ag_auth_data_unref(auth_data);
}

static AccountInfo *account_info_new(AccountWatcher *watcher, AgAccountService *account_service) {
    AccountInfo *info = g_new0(AccountInfo, 1);
    info->watcher = watcher;
    info->account_service = g_object_ref(account_service);

    AgAccount *account = ag_account_service_get_account(account_service);
    g_object_get(account, "id", &info->account_id, NULL);

    return info;
}

static gboolean service_is_supported(AccountWatcher *watcher,
                                     const char *service_id)
{
    GSList *node = g_slist_find_custom(watcher->supported_services,
                                       service_id,
                                       (GCompareFunc)g_strcmp0);
    return node != NULL;
}

static gboolean account_watcher_setup(void *user_data) {
    AccountWatcher *watcher = (AccountWatcher *)user_data;

    /* Now check initial state */
    GList *enabled_accounts =
        ag_manager_get_enabled_account_services(watcher->manager);
    GList *old_services = g_hash_table_get_keys(watcher->services);

    /* Update the services table */
    GList *l;
    for (l = enabled_accounts; l != NULL; l = l->next) {
        AgAccountService *account_service = l->data;
        AgAccountId id = ag_account_service_get_account(account_service)->id;
        AgService *service = ag_account_service_get_service(account_service);
        const char *service_id = ag_service_get_name(service);

        if (!service_is_supported(watcher, service_id)) continue;

        char *key = g_strdup_printf("%d/%s", id, service_id);

        AccountInfo *info = g_hash_table_lookup(watcher->services, key);
        if (info) {
            GList *node = g_list_find_custom(old_services, key,
                                             (GCompareFunc)g_strcmp0);
            old_services = g_list_remove_link(old_services, node);
            g_free(key);
        } else {
            trace("adding account %s\n", key);
            info = account_info_new(watcher, account_service);
            g_hash_table_insert(watcher->services, key, info);
        }
        account_info_login(info);
    }
    g_list_free_full(enabled_accounts, g_object_unref);

    /* Remove from the table the accounts which are no longer enabled */
    for (l = old_services; l != NULL; l = l->next) {
        char *key = l->data;
        trace("removing account %s\n", key);
        g_hash_table_remove(watcher->services, key);
    }
    g_list_free(old_services);

    return G_SOURCE_REMOVE;
}

AccountWatcher *account_watcher_new(AccountEnabledCallback callback,
                                    void *user_data) {
    AccountWatcher *watcher = g_new0(AccountWatcher, 1);

    watcher->manager = ag_manager_new();
    watcher->services = g_hash_table_new_full(
        g_str_hash, g_str_equal, g_free, (GDestroyNotify)account_info_free);
    watcher->supported_services = NULL;
    watcher->callback = callback;
    watcher->user_data = user_data;

    return watcher;
}

void account_watcher_add_service(AccountWatcher *watcher,
                                 char *serviceId) {
    watcher->supported_services =
        g_slist_prepend(watcher->supported_services, serviceId);
}

void account_watcher_run(AccountWatcher *watcher) {
    /* Make sure main setup occurs within the mainloop thread */
    g_idle_add(account_watcher_setup, watcher);
}

struct refresh_info {
    AccountWatcher *watcher;
    AgAccountId account_id;
    char *service_name;
};

static void refresh_info_free(struct refresh_info *data) {
    g_free(data->service_name);
    g_free(data);
}

static gboolean account_watcher_refresh_cb(void *user_data) {
    struct refresh_info *data = (struct refresh_info *)user_data;

    char *key = g_strdup_printf("%d/%s", data->account_id, data->service_name);
    AccountInfo *info = g_hash_table_lookup(data->watcher->services, key);
    if (info != NULL) {
        account_info_login(info);
    }

    return G_SOURCE_REMOVE;
}

void account_watcher_refresh(AccountWatcher *watcher, unsigned int account_id,
                             const char *service_name) {
    struct refresh_info *data = g_new(struct refresh_info, 1);
    data->watcher = watcher;
    data->account_id = account_id;
    data->service_name = g_strdup(service_name);
    g_idle_add_full(G_PRIORITY_DEFAULT_IDLE, account_watcher_refresh_cb,
                    data, (GDestroyNotify)refresh_info_free);
}
