#include "pam_raw.h"

// Free a partly built reply. Answers may hold a password, so wipe them.
static void free_reply(struct pam_response *reply, int num_msg){
    for (int i = 0; i<num_msg; i++){
        if (reply[i].resp != NULL){
            explicit_bzero(reply[i].resp, strlen(reply[i].resp));
            free(reply[i].resp);
        }
    }
    free(reply);
}

int p_conv(int num_msg, const struct pam_message **msg,
            struct pam_response **resp, void *appdata_ptr){
    struct pam_response *reply = NULL;
    struct credentials *creds = (struct credentials *)appdata_ptr;

    if (num_msg <= 0 || num_msg > PAM_MAX_NUM_MSG){
        return PAM_CONV_ERR;
    }

    reply = calloc(num_msg, sizeof(struct pam_response));
    if (reply == NULL){
        return PAM_BUF_ERR;
    }

    for (int i = 0; i<num_msg; i++){
        reply[i].resp_retcode = 0;
        reply[i].resp = NULL;

        switch(msg[i]->msg_style){
            case PAM_PROMPT_ECHO_OFF:
                if (creds == NULL || creds->fprint || creds->password == NULL){
                    // fprintd mode, or a prompt after authentication
                    // has finished: there is no password to give.
                    reply[i].resp = NULL; // move on
                }else{
                    reply[i].resp = strdup(creds->password);
                    if (reply[i].resp == NULL){
                        free_reply(reply, num_msg);
                        return PAM_BUF_ERR;
                    }
                }
                break;
            case PAM_PROMPT_ECHO_ON:
                reply[i].resp = strdup("");
                if (reply[i].resp == NULL){
                    free_reply(reply, num_msg);
                    return PAM_BUF_ERR;
                }
                break;
            case PAM_ERROR_MSG:
                fprintf(stderr, "PAM_ERROR_MSG: %s\n", msg[i]->msg);
                break;
            case PAM_TEXT_INFO:
                fprintf(stderr, "PAM_TEXT_INFO: %s\n", msg[i]->msg);
                break;
        }
    }
    
    *resp = reply;
    return PAM_SUCCESS;
}

int authenticate(pam_handle_t **pamh, const char *user, const char *password, const char *tty, int fprint){
    int ret;
    int cred_set = 0;

    struct credentials creds;
    creds.username = user;
    creds.password = password;
    creds.fprint = fprint;

    struct pam_conv conv = {
        p_conv,
        &creds
    };

    *pamh = NULL;

    if (fprint){
        // if fprintd is available
        ret = pam_start("hexdm", creds.username, &conv, pamh);
    }else{
        ret = pam_start("hexdm_nofprint", creds.username, &conv, pamh);
    }

    if (ret != PAM_SUCCESS){
        // There is no handle to end when pam_start fails.
        *pamh = NULL;
        return ret;
    }

    if (tty != NULL && tty[0] != '\0'){
        // pam_set_item takes its own copy.
        pam_set_item(*pamh, PAM_TTY, tty);
    }

    ret = pam_set_item(*pamh, PAM_USER, creds.username);
    if (ret != PAM_SUCCESS){
        goto fail;
    }

    ret = pam_authenticate(*pamh, 0);
    if (ret != PAM_SUCCESS){
        goto fail;
    }
    ret = pam_acct_mgmt(*pamh, 0);
    if (ret != PAM_SUCCESS){
        goto fail;
    }

    struct passwd *pw = getpwnam(creds.username);
    if (!pw){
        ret = PAM_USER_UNKNOWN;
        goto fail;
    }
    uid_t uid = pw->pw_uid;

    char runtime_env[64];
    snprintf(runtime_env, sizeof(runtime_env),
            "XDG_RUNTIME_DIR=/run/user/%u", (unsigned int)uid);

    pam_putenv(*pamh, runtime_env);

    ret = pam_setcred(*pamh, PAM_ESTABLISH_CRED);
    if (ret != PAM_SUCCESS){
        goto fail;
    }
    cred_set = 1;

    ret = pam_open_session(*pamh, 0);
    if (ret != PAM_SUCCESS){
        goto fail;
    }

    // creds lives on this stack frame and points at a password the caller
    // frees when we return, but the handle (and so the conversation) lives
    // on until teardown. Swap in a conversation with no credentials so a
    // module that talks to us later cannot read freed memory.
    struct pam_conv quiet = {
        p_conv,
        NULL
    };
    pam_set_item(*pamh, PAM_CONV, &quiet);

    return PAM_SUCCESS;

fail:
    // One way out for every failure: undo what was done, end the handle,
    // and clear the caller's pointer so it can't be used again.
    if (cred_set){
        pam_setcred(*pamh, PAM_DELETE_CRED);
    }
    pam_end(*pamh, ret);
    *pamh = NULL;
    return ret;
}

int teardown(pam_handle_t *pamh){
    if (pamh == NULL){
        return PAM_SUCCESS;
    }

    // Undo authenticate() in reverse order, and always carry on to
    // pam_end: an error in one step must not leave the session open or
    // the handle allocated.
    int ret = pam_close_session(pamh, 0);
    int cred_ret = pam_setcred(pamh, PAM_DELETE_CRED);
    if (ret == PAM_SUCCESS){
        ret = cred_ret;
    }
    pam_end(pamh, ret);

    return ret;
}

const char *get_err_code(int errcode){
    return pam_strerror(NULL, errcode);
}
