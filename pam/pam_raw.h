#ifndef PAM_RAW_H
#define PAM_RAW_H

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
};

int authenticate(pam_handle_t **pamh, const char *user, const char *password, const char *tty);

int teardown(pam_handle_t *pamh);

const char *get_err_code(int errcode);

char *get_pam_env_as_string(pam_handle_t *pamh, char *delim);

#endif // PAM_RAW_H
