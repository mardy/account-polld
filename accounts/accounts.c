#include "_cgo_export.h"

AccountWatcher *watch_for_service_type(const char *service_type) {
    /* Transfer service names to hash table */
    if (FALSE) {
        /* The Go callback doesn't quite match the
         * AccountEnabledCallback function prototype, so we cast the
         * argument in the account_watcher_new() call below.
         *
         * This is just a check to see that the function still has the
         * prototype we expect.
         */
        void (*unused)(void *watcher,
                       unsigned int account_id, char *service_name,
                       GError *error, int enabled,
                       char **auth_data, uint auth_data_length,
                       void *user_data) = authCallback;
    }

    AccountWatcher *watcher = account_watcher_new(
        service_type, (AccountEnabledCallback)authCallback, NULL);
    return watcher;
}
