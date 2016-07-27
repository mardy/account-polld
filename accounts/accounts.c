#include "_cgo_export.h"

AccountWatcher *watch() {
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
                       unsigned int account_id, 
                       char *service_type, char *service_name,
                       GError *error, int enabled,
                       char *client_id, char *client_secret,
                       char *access_token, char *token_secret,
                       void *user_data) = authCallback;
    }

    AccountWatcher *watcher = account_watcher_new(
        (AccountEnabledCallback)authCallback, NULL);
    return watcher;
}
