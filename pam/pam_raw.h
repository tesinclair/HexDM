#ifndef PAM_RAW_H
#define PAM_RAW_H

#include <security/pam_appl.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>

enum {
    SUCCESS = 1,
    FAILURE = 0,
    AUTH_ERROR = -1,
    FAILED_FPRINTD_PROBE = -2
};

struct credentials{
    const char* username;
    const char* password;
};

int authenticate(const char *user, const char *password);

const char *get_err_code(int errcode);

#endif // PAM_RAW_H
