#include "pam_raw.h"

int p_conv(int num_msg, const struct pam_message **msg,
            struct pam_response **resp, void *appdata_ptr){
    struct pam_response *reply = NULL;
    struct credentials *creds = (struct credentials *)appdata_ptr;

    reply = calloc(num_msg, sizeof(struct pam_response));
    if (reply == NULL){
        return PAM_BUF_ERR;
    }

    for (int i = 0; i<num_msg; i++){
        reply[i].resp_retcode = 0;
        reply[i].resp = NULL;

        switch(msg[i]->msg_style){
            case PAM_PROMPT_ECHO_OFF:
                if (strncmp(creds->password, "", 1) == 0){
                    // fprintd mode
                    reply[i].resp = NULL; // move on
                }else{
                    reply[i].resp = strdup(creds->password);
                }
                break;
            case PAM_PROMPT_ECHO_ON:
                reply[i].resp = strdup("");
                break;
            case PAM_ERROR_MSG:
            case PAM_TEXT_INFO:
                break;
        }
    }
    
    *resp = reply;
    return PAM_SUCCESS;
}

int authenticate(pam_handle_t **pamh, const char *user, const char *password){
    int ret;
    
    struct credentials creds;
    creds.username = user;
    creds.password = password;

    struct pam_conv conv = {
        p_conv,
        &creds
    };

    if (strncmp(creds.password, "", 1) == 0){
        // if fprintd is available
        ret = pam_start("hexdm", creds.username, &conv, pamh);
    }else{
        ret = pam_start("hexdm_nofprint", creds.username, &conv, pamh);
    }

    if (ret != PAM_SUCCESS){
        return ret;
    }

    const char *tty = ttyname(STDIN_FILENO);
    if (tty != NULL){
        pam_set_item(*pamh, PAM_TTY, strdup(tty));
    }

    ret = pam_set_item(*pamh, PAM_USER, creds.username);
    if (ret != PAM_SUCCESS){
        return ret;
    }

    ret = pam_authenticate(*pamh, 0);
    if (ret != PAM_SUCCESS){
        return ret;
    }
    ret = pam_acct_mgmt(*pamh, 0);
    if (ret != PAM_SUCCESS){
        return ret;
    }

    struct passwd *pw = getpwnam(creds.username);
    if (!pw){
        return -1;
    }
    uid_t uid = pw->pw_uid;

    char runtime_env[64];
    snprintf(runtime_env, sizeof(runtime_env),
            "XDG_RUNTIME_DIR=/run/user/%d", uid);

    pam_putenv(*pamh, runtime_env);

    ret = pam_setcred(*pamh, PAM_ESTABLISH_CRED);
    if (ret != PAM_SUCCESS){
        return ret;
    }

    ret = pam_open_session(*pamh, 0);
    if (ret != PAM_SUCCESS){
        return ret;
    }

    return ret;
}

int teardown(pam_handle_t *pamh){
    if (pamh == NULL){
        return PAM_SUCCESS;
    }

    int ret;

    ret = pam_setcred(pamh, PAM_DELETE_CRED);
    if (ret != PAM_SUCCESS){
        return ret;
    }

    ret = pam_close_session(pamh, 0);
    if (ret != PAM_SUCCESS){
        return ret;
    }
    pam_end(pamh, ret);

    return ret;
}

const char *get_err_code(int errcode){
    return pam_strerror(NULL, errcode);
}

char *get_pam_env_as_string(pam_handle_t *pamh, char *delim){
    char **env = pam_getenvlist(pamh);
    if (env == NULL){
        return NULL;
    }

    char *concatenv = malloc(1024);
    if (concatenv == NULL){
        return NULL;
    }
    concatenv[0] = '\0';
    int concatenv_size = 1024;

    int idx = 0;
    while (env[idx] != NULL){
        if ((strlen(concatenv) + strlen(env[idx]) + strlen(delim) + 1) > concatenv_size){
            concatenv_size = concatenv_size * 2;
            concatenv = realloc(concatenv, concatenv_size);
        }
        strcat(concatenv, env[idx]);
        strcat(concatenv, delim);
        free(env[idx++]);
    }

    free(env);
    return concatenv;
}
