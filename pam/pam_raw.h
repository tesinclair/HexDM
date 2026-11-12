#ifndef PAM_RAW_H
#define PAM_RAW_H

#ifndef _GNU_SOURCE
#define _GNU_SOURCE // explicit_bzero
#endif

#include <security/pam_appl.h>
#include <stddef.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>
#include <pwd.h>
#include <sys/types.h>

struct credentials{
    const char* username;
    const char* password;
    int fprint; // fingerprint mode: never answer a password prompt
};

int authenticate(pam_handle_t **pamh, const char *user, const char *password, const char *tty, int fprint);

int teardown(pam_handle_t *pamh);

const char *get_err_code(int errcode);

// Zero len bytes of buf, then free it.
void wipe_free(char *buf, size_t len);

#endif // PAM_RAW_H
